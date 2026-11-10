package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// immutableCacheControl lets browsers and proxies cache a hashed asset forever.
// The URL changes whenever the file content changes, so the cached copy can
// never go stale.
const immutableCacheControl = "public, max-age=31536000, immutable"

// assetSet maps the logical names of the embedded static files to names that
// contain a hash of their content, and back again.
type assetSet struct {
	hashedByName    map[string]string
	logicalByHashed map[string]string
}

// loadStaticAssets hashes every file at the top level of fsys and builds the
// two-way name map. Subdirectories (such as fonts) keep their plain URLs.
func loadStaticAssets(fsys fs.FS) (*assetSet, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read static assets: %w", err)
	}

	set := &assetSet{
		hashedByName:    make(map[string]string, len(entries)),
		logicalByHashed: make(map[string]string, len(entries)),
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read static asset %q: %w", name, err)
		}
		sum := sha256.Sum256(content)
		hashed := hashedFilename(name, hex.EncodeToString(sum[:])[:8])
		set.hashedByName[name] = hashed
		set.logicalByHashed[hashed] = name
	}
	return set, nil
}

// URL returns the hashed URL for a logical asset name. Unknown names are an
// error so a template fails loudly instead of emitting a broken link.
func (a *assetSet) URL(name string) (string, error) {
	hashed, ok := a.hashedByName[name]
	if !ok {
		return "", fmt.Errorf("static: unknown asset %q", name)
	}
	return "/static/" + hashed, nil
}

// hashedFilename inserts the hash before the file extension.
func hashedFilename(name, hash string) string {
	ext := path.Ext(name)
	if ext == "" {
		return name + "." + hash
	}
	return strings.TrimSuffix(name, ext) + "." + hash + ext
}

// cacheHashedStatic serves a known hashed filename from the real file and
// marks the response as immutable. Every other path falls through to next, so
// unhashed URLs keep working with their existing headers.
func cacheHashedStatic(assets *assetSet, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logical, ok := assets.logicalByHashed[strings.TrimPrefix(r.URL.Path, "/static/")]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Cache-Control", immutableCacheControl)
		r = r.Clone(r.Context())
		r.URL.Path = "/static/" + logical
		r.URL.RawPath = ""
		next.ServeHTTP(immutableCacheWriter{ResponseWriter: w}, r)
	})
}

// immutableCacheWriter drops the immutable Cache-Control header on error
// responses. http.ServeContent writes 412 (failed preconditions) and 416
// (unsatisfiable range) itself, and a proxy must not cache either for a year.
type immutableCacheWriter struct {
	http.ResponseWriter
}

func (w immutableCacheWriter) WriteHeader(status int) {
	if status < 200 || status >= 400 {
		w.Header().Del("Cache-Control")
	}
	w.ResponseWriter.WriteHeader(status)
}
