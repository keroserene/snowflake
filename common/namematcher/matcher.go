package namematcher

import "strings"

func NewNameMatcher(rule string) NameMatcher {
	rule = strings.TrimSuffix(rule, "$")
	return NameMatcher{suffix: strings.TrimPrefix(rule, "^"), exact: strings.HasPrefix(rule, "^")}
}

func IsValidRule(rule string) bool {
	return strings.HasSuffix(rule, "$")
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
