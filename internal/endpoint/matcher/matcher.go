package matcher

import "strings"

// Matcher is the stable string-matching boundary used by compiled endpoint
// rules. The initial implementations are deliberately simple baselines; faster
// trie/Aho-Corasick/etc. implementations can replace them behind this interface.
type Matcher interface {
	Match(string) bool
}

type emptyMatcher struct{}

func (emptyMatcher) Match(string) bool { return false }

type exactMatcher struct {
	values map[string]struct{}
}

func NewExact(values []string) Matcher {
	values = normalize(values)
	if len(values) == 0 {
		return emptyMatcher{}
	}
	m := exactMatcher{values: make(map[string]struct{}, len(values))}
	for _, value := range values {
		m.values[value] = struct{}{}
	}
	return m
}

func (m exactMatcher) Match(value string) bool {
	_, ok := m.values[value]
	return ok
}

type prefixMatcher struct {
	prefixes []string
}

func NewPrefix(prefixes []string) Matcher {
	prefixes = normalize(prefixes)
	if len(prefixes) == 0 {
		return emptyMatcher{}
	}
	return prefixMatcher{prefixes: prefixes}
}

func (m prefixMatcher) Match(value string) bool {
	for _, prefix := range m.prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

type suffixMatcher struct {
	suffixes []string
}

func NewSuffix(suffixes []string) Matcher {
	suffixes = normalize(suffixes)
	if len(suffixes) == 0 {
		return emptyMatcher{}
	}
	return suffixMatcher{suffixes: suffixes}
}

func (m suffixMatcher) Match(value string) bool {
	for _, suffix := range m.suffixes {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

type containsMatcher struct {
	patterns []string
}

func NewContains(patterns []string) Matcher {
	patterns = normalize(patterns)
	if len(patterns) == 0 {
		return emptyMatcher{}
	}
	return containsMatcher{patterns: patterns}
}

func (m containsMatcher) Match(value string) bool {
	for _, pattern := range m.patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func normalize(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
