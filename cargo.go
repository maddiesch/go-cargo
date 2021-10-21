package cargo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"time"
)

// DownloadInput provides the needed input for downloading a file.
type DownloadInput struct {
	// Source the URL that the file will be downloaded from. It is a required
	// value for input.
	Source *url.URL

	// Dest is the Writer that the downloaded data will be written to. In the case
	// of Cargo, this will usually be a *os.File
	Dest io.Writer

	// Optional *http.Client used to send the request. Defaults to
	// http.DefaultClient if no value is specified.
	HTTPClient *http.Client

	// Optional function used to create the HTTP request for the given URL. If no
	// function is set a default request will be created using the HTTP method
	// "GET"
	CreateRequest func(context.Context, *url.URL) (*http.Request, error)

	// Optional function that can be used to valid a HTTP response. By default no
	// status code validation is performed and the response body is written to the
	// destination.
	ValidateResponse func(*http.Response) error

	// Optional handler for processing response progress updates. By default there
	// is no progress reporting.
	ProgressHandler ProgressHandler

	// Optional value for controlling the download read & copy to the temporary
	// destination. If there is no timeout specified a value of 1 hour will be
	// used.
	ReadTimeout time.Duration

	// Optional value for controlling the copy to the destination writer. If there
	// is no timeout specified a value of 1 hour will be used.
	CopyTimeout time.Duration
}

// DownloadOutput contains metadata about the download. It can safely be ignored
// as any failures will be returned in the error result.
type DownloadOutput struct {
	FileSize int64         // Final size of the downloaded file
	Duration time.Duration // Full download time
}

// Download executes a download from the URL.
//
// The file will be downloaded to a temp file, before being copied into the
// input's Dest writer. This is to ensure that a network error will not cause
// the destination to be overwritten by bad data.
func Download(ctx context.Context, in DownloadInput) (*DownloadOutput, error) {
	errChan := make(chan error, 1)
	doneChan := make(chan *DownloadOutput, 1)

	if in.CreateRequest == nil {
		in.CreateRequest = func(ctx context.Context, u *url.URL) (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, `GET`, u.String(), nil)
			if err != nil {
				return nil, err
			}

			req.Header.Set("User-Agent", "Go-Cargo (github.com/maddiesch/go-cargo)")

			return req, nil
		}
	}
	if in.HTTPClient == nil {
		in.HTTPClient = http.DefaultClient
	}
	if in.ReadTimeout == 0 {
		in.ReadTimeout = 1 * time.Hour
	}
	if in.CopyTimeout == 0 {
		in.CopyTimeout = 1 * time.Hour
	}

	go func() {
		defer close(errChan)
		defer close(doneChan)

		startTime := time.Now()

		checkCtxAndFailIfCanceled := func(ctx context.Context) {
			if err := ctx.Err(); err != nil {
				errChan <- err
				runtime.Goexit()
			}
		}

		failWithErr := func(err error) {
			errChan <- err
			runtime.Goexit()
		}

		checkCtxAndFailIfCanceled(ctx)

		req, err := in.CreateRequest(ctx, in.Source)
		if err != nil {
			failWithErr(err)
		}

		checkCtxAndFailIfCanceled(ctx)

		resp, err := in.HTTPClient.Do(req)
		if err != nil {
			failWithErr(err)
		}

		if in.ValidateResponse != nil {
			if err := in.ValidateResponse(resp); err != nil {
				failWithErr(err)
			}
		}

		checkCtxAndFailIfCanceled(ctx)

		contentLen := contentLengthFromResponse(resp)
		if in.ProgressHandler != nil {
			in.ProgressHandler.Expected(contentLen)
		}

		tmpFile, err := os.CreateTemp("", "cargo-download-*")
		if err != nil {
			failWithErr(err)
		}
		defer func() {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
		}()

		readProgress := createProgressWriter(in.ProgressHandler)

		readCtx, readCancel := context.WithTimeout(ctx, in.ReadTimeout)
		defer readCancel()

		if _, err := copyWithContext(readCtx, tmpFile, io.TeeReader(resp.Body, readProgress)); err != nil {
			failWithErr(err)
		}

		checkCtxAndFailIfCanceled(ctx)

		if _, err := tmpFile.Seek(0, 0); err != nil {
			failWithErr(err)
		}

		copyCtx, copyCancel := context.WithTimeout(ctx, in.CopyTimeout)
		defer copyCancel()

		finalSize, err := copyWithContext(copyCtx, in.Dest, tmpFile)
		if err != nil {
			failWithErr(err)
		}

		doneChan <- &DownloadOutput{
			FileSize: int64(finalSize),
			Duration: time.Since(startTime),
		}
	}()

	select {
	case err := <-errChan:
		return nil, err
	case out := <-doneChan:
		return out, nil
	}
}

var (
	errInvalidWrite = errors.New(`invalid write`)
)

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64

CopyLoop:
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}

		nr, rErr := src.Read(buf)

		if nr > 0 {
			nw, wErr := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if wErr == nil {
					wErr = errInvalidWrite
				}
			}
			written += int64(nw)
			if wErr != nil {
				return written, wErr
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}

		if rErr != nil {
			if errors.Is(rErr, io.EOF) {
				break CopyLoop
			}
			return written, rErr
		}
	}

	return written, nil
}

func createProgressWriter(h ProgressHandler) io.Writer {
	if h == nil {
		return io.Discard
	}

	return &progresWriter{h}
}

type progresWriter struct {
	h ProgressHandler
}

func (w *progresWriter) Write(b []byte) (int, error) {
	n := len(b)

	w.h.Receive(n)

	return n, nil
}

func contentLengthFromResponse(r *http.Response) int64 {
	unsafeHeaderString := r.Header.Get(`Content-Length`)
	len, err := strconv.ParseInt(unsafeHeaderString, 10, 64)
	if err != nil {
		return -1
	}
	return len
}

// HTTPResponseError is the error returned when an HTTP response contains an
// invalid status code.
type HTTPResponseError struct {
	StatusCode int
}

func (e *HTTPResponseError) Error() string {
	return fmt.Sprintf("http response error (%s)", http.StatusText(e.StatusCode))
}

// ValidateStatusCodeEqual returns a function for DownloadInput.ValidateResponse
// that verifies the response's status code is equal to the given status code.
// If the values are not equal a HTTPResponseError will be returned.
func ValidateStatusCodeEqual(status int) func(*http.Response) error {
	return func(r *http.Response) error {
		if r.StatusCode == status {
			return nil
		}
		return &HTTPResponseError{r.StatusCode}
	}
}
