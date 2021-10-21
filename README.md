# Cargo

HTTP file downloads with progress reporting.

```go
source, _ := url.Parse(`https://...`)

file, _ := os.CreateTemp("", "download-*")
defer file.Close()

in := cargo.DownloadInput{
  Source: source,
  Dest:   file,
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

fmt.Printf("Downloaded to %s\n", file.Name())
```
