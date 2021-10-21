package cargo_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/maddiesch/go-cargo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownload(t *testing.T) {
	source, _ := url.Parse(`https://cdn.maddie.cloud/random-data/rand_16k.dat`)

	t.Run(`given a valid URL and destination`, func(t *testing.T) {
		f, _ := os.Create(tempFilePath(t.Name()) + `.dat`)
		defer f.Close()

		var expectedSize, progressSize int64

		in := cargo.DownloadInput{
			Source:           source,
			Dest:             f,
			ValidateResponse: cargo.ValidateStatusCodeEqual(http.StatusOK),
			ProgressHandler: cargo.ProgressHandlerFunc(func(ex, to int64) {
				expectedSize = ex
				progressSize = to
			}),
		}

		out, err := cargo.Download(context.Background(), in)

		require.NoError(t, err)

		assert.Equal(t, int64(16000), expectedSize)
		assert.Equal(t, int64(16000), progressSize)
		assert.Equal(t, int64(16000), out.FileSize)
	})
}

func ExampleDownload() {
	source, _ := url.Parse(`https://...`)

	file, _ := os.CreateTemp("", "download-*")
	defer file.Close()

	in := cargo.DownloadInput{
		Source:           source,
		Dest:             file,
		ValidateResponse: cargo.ValidateStatusCodeEqual(http.StatusOK),
		ProgressHandler: cargo.ProgressHandlerFunc(func(totalExpected, totalReceived int64) {
			fmt.Printf("Downloaded %d of %d bytes\n", totalExpected, totalReceived)
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := cargo.Download(ctx, in)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Downloaded to %s", file.Name())
}

func tempFilePath(parts ...string) string {
	_, p, _, _ := runtime.Caller(0)

	path := filepath.Join(filepath.Dir(p), `tmp`, filepath.Join(parts...))

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		panic(err)
	}

	return path
}
