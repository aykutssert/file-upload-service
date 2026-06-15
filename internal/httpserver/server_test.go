package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRunDrainsActiveRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := &http.Server{
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			close(requestStarted)
			<-releaseRequest
			writer.WriteHeader(http.StatusNoContent)
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- Run(
			ctx,
			server,
			listener,
			time.Second,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)
	}()

	responseDone := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			responseDone <- err
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			responseDone <- &unexpectedStatusError{status: response.Status}
			return
		}
		responseDone <- nil
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}

	cancel()

	select {
	case err := <-runDone:
		t.Fatalf("server stopped before active request completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseRequest)

	if err := <-responseDone; err != nil {
		t.Fatalf("active request: %v", err)
	}
	if err := <-runDone; err != nil {
		t.Fatalf("run server: %v", err)
	}
}

type unexpectedStatusError struct {
	status string
}

func (err *unexpectedStatusError) Error() string {
	return "unexpected status: " + err.status
}
