package agent

import (
	"regexp"
	"strconv"
	"strings"
)

var semverPattern = regexp.MustCompile(`(?i)(?:^|[^0-9a-z])v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:[^0-9]|$)`)

// isNewer reports whether the latest comparable semantic version is newer.
func isNewer(latest, current string) bool {
	_, latestOK := extractComparableVersion(latest)
	_, currentOK := extractComparableVersion(current)
	if current == "dev" || current == "" || !currentOK {
		return latestOK
	}
	cmp, ok := CompareVersions(latest, current)
	return ok && cmp > 0
}

// CompareVersions compares two semver-like strings after extracting a comparable
// version from each label. It supports plain semver values, prefixed tags, and
// artifact-like labels that embed a semantic version. The boolean return value
// reports whether both values were comparable.
func CompareVersions(a, b string) (int, bool) {
	av, aOK := extractComparableVersion(a)
	bv, bOK := extractComparableVersion(b)
	if !aOK || !bOK {
		return 0, false
	}
	return compareComparableVersions(av, bv), true
}

// IsVersionBlockedByRollback reports whether targetVersion should be skipped by
// the rollback high-watermark. Exact version matches are always blocked. When
// both values are comparable semver-like labels, versions <= maxIgnoredVersion
// are also blocked.
func IsVersionBlockedByRollback(targetVersion, maxIgnoredVersion string) bool {
	targetVersion = strings.TrimSpace(targetVersion)
	maxIgnoredVersion = strings.TrimSpace(maxIgnoredVersion)
	if targetVersion == "" || maxIgnoredVersion == "" {
		return false
	}
	if targetVersion == maxIgnoredVersion {
		return true
	}
	cmp, ok := CompareVersions(targetVersion, maxIgnoredVersion)
	return ok && cmp <= 0
}

// RollbackHighWatermark returns the version label that should be remembered
// after a manual rollback so future polling can avoid immediately re-applying
// the same bad version. When both versions are comparable, the previous version
// is pinned only if the rollback target is older. For non-comparable labels,
// Watcher conservatively pins the previous exact label so exact rediscovery is
// still blocked.
func RollbackHighWatermark(targetVersion, previousVersion string) string {
	targetVersion = strings.TrimSpace(targetVersion)
	previousVersion = strings.TrimSpace(previousVersion)
	if targetVersion == "" || previousVersion == "" || targetVersion == previousVersion {
		return ""
	}
	cmp, ok := CompareVersions(targetVersion, previousVersion)
	if !ok {
		return previousVersion
	}
	if cmp < 0 {
		return previousVersion
	}
	return ""
}

// extractComparableVersion extracts the first comparable semantic version from free-form text.
func extractComparableVersion(raw string) ([3]int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return [3]int{}, false
	}

	matches := semverPattern.FindStringSubmatch(raw)
	if len(matches) != 4 {
		if strings.HasPrefix(strings.ToLower(raw), "v") {
			matches = semverPattern.FindStringSubmatch(" " + raw)
		}
		if len(matches) != 4 {
			return [3]int{}, false
		}
	}

	var version [3]int
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(matches[i+1])
		if err != nil {
			return [3]int{}, false
		}
		version[i] = n
	}
	return version, true
}

// compareComparableVersions orders two parsed semantic-version tuples.
func compareComparableVersions(latest, current [3]int) int {
	for i := 0; i < 3; i++ {
		if latest[i] > current[i] {
			return 1
		}
		if latest[i] < current[i] {
			return -1
		}
	}
	return 0
}
