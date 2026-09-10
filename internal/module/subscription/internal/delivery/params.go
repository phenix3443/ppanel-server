package delivery

// mergeParams layers the subscription URL's query string over the client
// application's stored defaults, so a template can rely on its own defaults while
// any single one of them stays overridable per request.
func mergeParams(defaults, requested map[string]string) map[string]string {
	if len(defaults) == 0 {
		return requested
	}
	merged := make(map[string]string, len(defaults)+len(requested))
	for key, value := range defaults {
		merged[key] = value
	}
	for key, value := range requested {
		merged[key] = value
	}
	return merged
}
