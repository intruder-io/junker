package main

func generateMutations() map[string]string {
	m := make(map[string]string, 0)

	m["identity"] = "Content-Length: %s"

	m["colon-prefix-chars"] = "Content-Length abcd: %s"
	m["colon-prefix-space"] = "Content-Length : %s"
	m["colon-prefix-null"] = "Content-Length\x00: %s"
	m["colon-prefix-tab"] = "Content-Length\t: %s"
	m["colon-prefix-vtab"] = "Content-Length\x0b: %s"
	m["colon-prefix-cr"] = "Content-Length\r: %s"

	m["colon-post-chars"] = "Content-Length:abcd %s"
	m["colon-post-null"] = "Content-Length:\x00%s"
	m["colon-post-tab"] = "Content-Length:\t%s"
	m["colon-post-vtab"] = "Content-Length:\x0b%s"
	m["colon-post-cr"] = "Content-Length:\r%s"

	m["line-prefix-space"] = " Content-Length: %s"
	m["line-prefix-tab"] = "\tContent-Length: %s"
	m["line-prefix-vtab"] = "\x0bContent-Length: %s"
	m["line-prefix-null"] = "\x00Content-Length: %s"
	m["line-prefix-cr"] = "\rContent-Length: %s"

	m["cr"] = "X-Header: y\rContent-Length: %s"
	m["double-cr"] = "X-Header: y\r\rContent-Length: %s"

	m["nospace"] = "Content-Length:%s"
	m["uppercase"] = "CONTENT-LENGTH: %s"
	m["hex"] = "Content-Length: 0x%s"

	// Amit Klein's "challenges"
	m["cr-hyphenated"] = "Content\rLength: %s"
	m["signed"] = "Content-Length: +%s"

	return m
}
