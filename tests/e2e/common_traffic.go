package e2e

import (
	"bytes"
	"fmt"
)

type HTTPRequestTest struct {
	sourceNs        string
	sourcePod       string
	sourceContainer string

	destNs       string
	destHostname string // Resolvable IP

	httpUrl string
	port    int
}

// HTTPRequest runs a basic HTTP resquest from the source container to the expected destination
// Returns Status code and map of headers with their value, if no err
func (td *OsmTestData) HTTPRequest(ht HTTPRequestTest) (int, map[string]string, error) {
	var stdin, stdout, stderr bytes.Buffer

	// -s silent, -o output to devnull, "-D -" dump headers to file (- stdout), -i Status code
	command := fmt.Sprintf("/usr/bin/curl -s -o /dev/null -D - -i http://%s.%s:%d%s", ht.destHostname, ht.destNs, ht.port, ht.httpUrl)

	err := td.RunRemote(ht.sourceNs, ht.sourcePod, ht.sourceContainer, command, &stdin, &stdout, &stderr)
	if err != nil {
		td.T.Logf("Error running command")
		return 0, map[string]string{}, err
	}
	td.T.Logf("%v", stdout.String())
	return 0, map[string]string{}, nil
}
