package server_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Mic92/niks3/client"
	"github.com/minio/minio-go/v7"
)

// Paths signed by cache.nixos.org-1 are left to the upstream cache: a push
// uploads the paths that depend on them, but not the paths themselves.
func TestClientSkipsUpstreamPaths(t *testing.T) {
	t.Parallel()

	testService := createTestServiceWithAuth(t, testAuthToken)
	t.Cleanup(func() { testService.Close() })

	ok(t, testService.InitializeBucket(t.Context()))

	mux := http.NewServeMux()
	registerTestHandlers(mux, testService)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	ctx := t.Context()
	nixEnv := setupIsolatedNixStore(t)

	dep := `builtins.toFile "upstream-dep" "from cache.nixos.org"`
	depPath := nixEvalStorePath(t, nixEnv, dep)
	ownPath := nixEvalStorePath(t, nixEnv, `builtins.toFile "own" "${`+dep+`}"`)

	// A local key that has the upstream key's name stands in for the real one.
	keyFile := filepath.Join(t.TempDir(), "upstream.sec")

	genKey := exec.CommandContext(ctx, "nix", "--extra-experimental-features", "nix-command",
		"key", "generate-secret", "--key-name", "cache.nixos.org-1")
	genKey.Env = nixEnv
	key, err := genKey.Output()
	ok(t, err)
	ok(t, os.WriteFile(keyFile, key, 0o600))

	sign := exec.CommandContext(ctx, "nix", "--extra-experimental-features", "nix-command",
		"store", "sign", "--key-file", keyFile, depPath)
	sign.Env = nixEnv
	ok(t, sign.Run())

	c, err := client.NewClient(ctx, ts.URL, testAuthToken)
	ok(t, err)

	c.NixEnv = nixEnv

	_, err = c.PushPaths(ctx, []string{ownPath})
	ok(t, err)

	ownHash, err := client.GetStorePathHash(ownPath)
	ok(t, err)
	verifyNarinfoInS3(ctx, t, testService, ownHash, ownPath)

	depHash, err := client.GetStorePathHash(depPath)
	ok(t, err)

	if _, err := testService.MinioClient.StatObject(ctx, testService.Bucket, depHash+".narinfo", minio.StatObjectOptions{}); err == nil {
		t.Errorf("upstream path %s was uploaded", depPath)
	} else if code := minio.ToErrorResponse(err).Code; code != "NoSuchKey" {
		t.Errorf("stat %s.narinfo: %v", depHash, err)
	}

	// Pushing only upstream paths is a successful no-op.
	_, err = c.PushPaths(ctx, []string{depPath})
	ok(t, err)
}
