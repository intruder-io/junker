package main

func generateMutations() map[string]string {
	m := make(map[string]string, 0)

	m["id"] = "Content-Length: %s"
	m["colon-prefix-chars"] = "Content-Length abcd: %s"

	return m
}
