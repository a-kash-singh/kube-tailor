package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/a-kash-singh/kube-tailor/pkg/admission"
	"github.com/a-kash-singh/kube-tailor/pkg/mutation"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

func main() {
	setLogger()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mutator := buildMutator(ctx)

	http.HandleFunc("/validate-pods", ServeValidatePods)
	http.HandleFunc("/mutate-pods", serveMutatePods(mutator))
	http.HandleFunc("/health", ServeHealth)

	if os.Getenv("TLS") == "true" {
		cert := "/etc/admission-webhook/tls/tls.crt"
		key := "/etc/admission-webhook/tls/tls.key"
		logrus.Print("Listening on port 443...")
		logrus.Fatal(http.ListenAndServeTLS(":443", cert, key, nil))
	} else {
		logrus.Print("Listening on port 8080...")
		logrus.Fatal(http.ListenAndServe(":8080", nil))
	}
}

// buildMutator creates a shared Mutator backed by an informer cache.
// If an in-cluster config is unavailable (local dev), it falls back to a
// mutator that will log a warning and skip resource injection.
func buildMutator(ctx context.Context) *mutation.Mutator {
	logger := logrus.WithField("component", "startup")

	config, err := rest.InClusterConfig()
	if err != nil {
		logger.WithError(err).Warn("not running in-cluster; resource injection will be disabled")
		return mutation.NewMutator(logrus.NewEntry(logrus.StandardLogger()))
	}

	client, factory, err := mutation.NewKubeClientFromConfig(config)
	if err != nil {
		logger.WithError(err).Fatal("failed to build Kubernetes client")
	}

	factory.Start(ctx.Done())

	logger.Info("waiting for informer cache to sync...")
	synced := cache.WaitForCacheSync(
		ctx.Done(),
		factory.Core().V1().Nodes().Informer().HasSynced,
		factory.Apps().V1().DaemonSets().Informer().HasSynced,
	)
	if !synced {
		logger.Fatal("informer cache failed to sync — shutting down")
	}
	logger.Info("informer cache ready")

	return mutation.NewMutatorWithClient(logrus.NewEntry(logrus.StandardLogger()), client)
}

// ServeHealth returns 200 when things are good
func ServeHealth(w http.ResponseWriter, r *http.Request) {
	logrus.WithField("uri", r.RequestURI).Debug("healthy")
	fmt.Fprint(w, "OK")
}

// ServeValidatePods validates an admission request and then writes an admission
// review to `w`
func ServeValidatePods(w http.ResponseWriter, r *http.Request) {
	logger := logrus.WithField("uri", r.RequestURI)
	logger.Debug("received validation request")

	in, err := parseRequest(*r)
	if err != nil {
		logger.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	adm := admission.Admitter{
		Logger:  logger,
		Request: in.Request,
	}

	out, err := adm.ValidatePodReview()
	if err != nil {
		e := fmt.Sprintf("could not generate admission response: %v", err)
		logger.Error(e)
		http.Error(w, e, http.StatusInternalServerError)
		return
	}

	writeAdmissionResponse(w, logger, out)
}

// serveMutatePods returns an HTTP handler that uses the shared mutator.
func serveMutatePods(mutator *mutation.Mutator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logrus.WithField("uri", r.RequestURI)
		logger.Debug("received mutation request")

		in, err := parseRequest(*r)
		if err != nil {
			logger.Error(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		adm := admission.Admitter{
			Logger:  logger,
			Request: in.Request,
			Mutator: mutator,
		}

		out, err := adm.MutatePodReview()
		if err != nil {
			e := fmt.Sprintf("could not generate admission response: %v", err)
			logger.Error(e)
			http.Error(w, e, http.StatusInternalServerError)
			return
		}

		writeAdmissionResponse(w, logger, out)
	}
}

func writeAdmissionResponse(w http.ResponseWriter, logger *logrus.Entry, out *admissionv1.AdmissionReview) {
	w.Header().Set("Content-Type", "application/json")
	jout, err := json.Marshal(out)
	if err != nil {
		e := fmt.Sprintf("could not parse admission response: %v", err)
		logger.Error(e)
		http.Error(w, e, http.StatusInternalServerError)
		return
	}
	logger.Debug("sending response")
	logger.Debugf("%s", jout)
	fmt.Fprintf(w, "%s", jout)
}

// setLogger sets the logger using env vars, it defaults to text logs on
// debug level unless otherwise specified
func setLogger() {
	logrus.SetLevel(logrus.DebugLevel)

	lev := os.Getenv("LOG_LEVEL")
	if lev != "" {
		llev, err := logrus.ParseLevel(lev)
		if err != nil {
			logrus.Fatalf("cannot set LOG_LEVEL to %q", lev)
		}
		logrus.SetLevel(llev)
	}

	if os.Getenv("LOG_JSON") == "true" {
		logrus.SetFormatter(&logrus.JSONFormatter{})
	}
}

// parseRequest extracts an AdmissionReview from an http.Request if possible
func parseRequest(r http.Request) (*admissionv1.AdmissionReview, error) {
	if r.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("Content-Type: %q should be %q",
			r.Header.Get("Content-Type"), "application/json")
	}

	bodybuf := new(bytes.Buffer)
	bodybuf.ReadFrom(r.Body)
	body := bodybuf.Bytes()

	if len(body) == 0 {
		return nil, fmt.Errorf("admission request body is empty")
	}

	var a admissionv1.AdmissionReview

	if err := json.Unmarshal(body, &a); err != nil {
		return nil, fmt.Errorf("could not parse admission review request: %v", err)
	}

	if a.Request == nil {
		return nil, fmt.Errorf("admission review can't be used: Request field is nil")
	}

	return &a, nil
}
