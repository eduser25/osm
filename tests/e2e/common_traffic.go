package e2e

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

const (
	// HStatusCodeWord is a hardcoded word to use on curl command, to print and parse REST Status code
	StatusCodeWord = "StatusCode"
)

// HTTPRequestDef defines a remote HTTP request intent
type HTTPRequestDef struct {
	SourceNs        string
	SourcePod       string
	SourceContainer string

	Destination string // Either host or IP

	HTTPUrl string
	Port    int
}

// HTTPRequest runs a basic HTTP resquest from the source container to the expected destination
func (td *OsmTestData) HTTPRequest(ht HTTPRequestDef) (int, map[string]string, error) {
	// -s silent progress, -o output to devnull, '-D -' dump headers to "-" (stdout), -i Status code
	// -I skip body download, '-w StatusCode:%{http_code}' prints Status code label-liked for easy parsing
	command := fmt.Sprintf("/usr/bin/curl -s -o /dev/null -D - -I -w %s:%%{http_code} http://%s:%d%s", StatusCodeWord, ht.Destination, ht.Port, ht.HTTPUrl)

	td.T.Logf("- (Curl) HTTP GET %s/%s[%s] -> http://%s:%d%s", ht.SourceNs, ht.SourcePod, ht.SourceContainer, ht.Destination, ht.Port, ht.HTTPUrl)
	stdout, stderr, err := td.RunRemote(ht.SourceNs, ht.SourcePod, ht.SourceContainer, command)
	if err != nil {
		// Error codes from the execution will come in err
		return 0, nil, fmt.Errorf("Remote exec err: %v | stderr: %s", err, stderr)
	}
	if len(stderr) > 0 {
		// no error from execution and proper exit code, but we got some stderr
		td.T.Logf("[warn] Stderr: %v", stderr)
	}

	// Expecting predictable output at this point
	curlMappedReturn := mapCurlOuput(stdout)

	statusCode, err := strconv.Atoi(curlMappedReturn[StatusCodeWord])
	if err != nil {
		return 0, nil, fmt.Errorf("Could not read status code as integer: %v", err)
	}
	delete(curlMappedReturn, StatusCodeWord)

	return statusCode, curlMappedReturn, nil
}

// MapCurlOuput maps predictable stdout from our specific curl, which will
// output reply headers in some form of "<name>: <value>"
func mapCurlOuput(curlOut string) map[string]string {
	var ret map[string]string = make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(curlOut))

	for scanner.Scan() {
		line := scanner.Text()

		// Expect at most 2 substrings, separating by only the first colon
		splitResult := strings.SplitN(line, ":", 2)

		if len(splitResult) != 2 {
			// other non-header data
			continue
		}
		ret[strings.TrimSpace(splitResult[0])] = strings.TrimSpace(splitResult[1])
	}
	return ret
}
