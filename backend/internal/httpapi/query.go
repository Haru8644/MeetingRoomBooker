package httpapi

import (
	"net/url"
	"strings"
)

func firstQueryValue(query, name string) (string, bool, error) {
	for parameter := range strings.SplitSeq(query, "&") {
		key, rawValue, _ := strings.Cut(parameter, "=")
		field, err := url.QueryUnescape(key)
		if err == nil && strings.EqualFold(field, name) {
			value, err := url.QueryUnescape(rawValue)
			if err != nil {
				return rawValue, true, err
			}
			return value, true, nil
		}
	}
	return "", false, nil
}
