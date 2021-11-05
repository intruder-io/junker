A CL.CL request smuggling scanner which uses invalid values in the "Content-Length" header for detection.

# Usage
```
$ junker -h
Usage of junker:
  -b, --batch-size int     The number of input lines to process at once, shuffling the tests for each (default 200)
  -H, --headers strings    Extra headers to include in requests - to add multiple headers specify multiple times
  -m, --methods strings    HTTP method to test - to test multiple methods specify multiple times (default [POST])
  -n, --no-resolve         Don't resolve domains, instead expect the input to be in the format <ip>,<url>
  -o, --output string      The file to output results to, in JSON format - use '-' for stdout (default "junker.json")
  -r, --runs int           The number of times to check before flagging a result as vulnerable (default 3)
  -t, --timeout duration   The timeout value to use for HTTP requests (default 5s)
  -c, --workers int        The number of concurrent wokers (default 10)
```
