package client

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
)

// upstreamCacheKeyName is the signing key of the upstream cache. Like Attic's
// upstream_cache_key_names, paths it signed are not pushed.
const upstreamCacheKeyName = "cache.nixos.org-1"

// isUpstreamPath reports whether upstreamCacheKeyName signed info.
func isUpstreamPath(info *PathInfo) bool {
	return slices.ContainsFunc(info.Signatures, func(sig string) bool {
		return strings.HasPrefix(sig, upstreamCacheKeyName+":")
	})
}

// filterUpstreamPaths drops paths that upstream caches already serve from the
// top-level paths and pathInfos. Narinfos of the kept paths still list them
// as references, and substituters fetch them from the upstream cache.
func filterUpstreamPaths(topLevelPaths []string, pathInfos map[string]*PathInfo) ([]string, map[string]*PathInfo) {
	kept := make(map[string]*PathInfo, len(pathInfos))

	for path, info := range pathInfos {
		if !isUpstreamPath(info) {
			kept[path] = info
		}
	}

	if skipped := len(pathInfos) - len(kept); skipped > 0 {
		slog.Info(fmt.Sprintf("Skipping %d paths signed by an upstream cache", skipped))
	}

	return slices.DeleteFunc(slices.Clone(topLevelPaths), func(p string) bool {
		_, ok := kept[p]

		return !ok
	}), kept
}
