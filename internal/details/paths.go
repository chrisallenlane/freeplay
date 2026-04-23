package details

// memKey builds the in-memory cache key for (console, cleanName).
func memKey(console, cleanName string) string {
	return console + "/" + cleanName
}
