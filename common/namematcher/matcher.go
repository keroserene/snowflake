package namematcher

import "strings"

func NewNameMatcher(rule string) NameMatcher {
	return NameMatcher{suffix: strings.TrimPrefix(rule, "^"), exact: strings.HasPrefix(rule, "^")}
}

type NameMatcher struct {
	exact  bool
	suffix string
}

func (m *NameMatcher) IsSupersetOf(matcher NameMatcher) bool {
	if m.exact {
		return matcher.exact && m.suffix == matcher.suffix
	}
	return strings.HasSuffix(matcher.suffix, m.suffix)
}

func (m *NameMatcher) IsMember(s string) bool {
	if m.exact {
		return s == m.suffix
	}
	return strings.HasSuffix(s, m.suffix)
}
