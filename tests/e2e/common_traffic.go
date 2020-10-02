package e2e

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/gomega"

	"github.com/fatih/color"
)

const (
	// StatusCodeWord is a hardcoded word to use on curl command, to print and parse REST Status code
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

// HTTPRequestResult represents results of an HTTPRequest call
type HTTPRequestResult struct {
	StatusCode int
	Headers    map[string]string
	Err        error
}

// HTTPRequest runs a basic HTTP resquest from the source ns/pod/container to the a given host destination
// Returns Status code, map with HTTP headers and error if any
func (td *OsmTestData) HTTPRequest(ht HTTPRequestDef) HTTPRequestResult {
	// -s silent progress, -o output to devnull, '-D -' dump headers to "-" (stdout), -i Status code
	// -I skip body download, '-w StatusCode:%{http_code}' prints Status code label-like for easy parsing
	command := fmt.Sprintf("/usr/bin/curl -s -o /dev/null -D - -I -w %s:%%{http_code} http://%s:%d%s", StatusCodeWord, ht.Destination, ht.Port, ht.HTTPUrl)

	//td.T.Logf("- (Curl) HTTP GET %s/%s[%s] -> http://%s:%d%s", ht.SourceNs, ht.SourcePod, ht.SourceContainer, ht.Destination, ht.Port, ht.HTTPUrl)
	stdout, stderr, err := td.RunRemote(ht.SourceNs, ht.SourcePod, ht.SourceContainer, command)
	if err != nil {
		// Error codes from the execution come through err
		// Curl 'Connection refused' err code = 7
		return HTTPRequestResult{
			0,
			nil,
			fmt.Errorf("Remote exec err: %v | stderr: %s", err, stderr),
		}
	}
	if len(stderr) > 0 {
		// no error from execution and proper exit code, we got some stderr though
		td.T.Logf("[warn] Stderr: %v", stderr)
	}

	// Expecting predictable output at this point
	curlMappedReturn := mapCurlOuput(stdout)
	statusCode, err := strconv.Atoi(curlMappedReturn[StatusCodeWord])
	if err != nil {
		return HTTPRequestResult{
			0,
			nil,
			fmt.Errorf("Could not read status code as integer: %v", err),
		}
	}
	delete(curlMappedReturn, StatusCodeWord)

	return HTTPRequestResult{
		statusCode,
		curlMappedReturn,
		nil,
	}
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

// HTTPAsyncResults are a storage type for async results
type HTTPAsyncResults struct {
	mux        *sync.Mutex // Requied to read results by the caller, as they can updating
	lastResult HTTPRequestResult
}

// NewHTTPAsyncResults returns a properly initialized async results instance
func NewHTTPAsyncResults() *HTTPAsyncResults {
	return &HTTPAsyncResults{
		mux: &sync.Mutex{},
		lastResult: HTTPRequestResult{
			Headers: map[string]string{},
		},
	}
}

// Convenience function
func (res HTTPAsyncResults) withLock(f func()) {
	res.mux.Lock()
	defer res.mux.Unlock()
	f()
}

// HTTPAsync is the context fed to a HTTP async thread to run HTTP queries.
// Stores Request definition, and pointers to results, stop and wait structures.
// Pointers are used to signal the fact that the calling application might
type HTTPAsync struct {
	// Request
	RequestData HTTPRequestDef

	// Async results
	ResponseDataStore *HTTPAsyncResults

	// Stop signaling
	StopSignal *bool
	WaitGroup  *sync.WaitGroup

	// Time sleep observed between requests
	SleepTime time.Duration
}

// AsynchronousHTTPRunner is a template function which will run HTTP request, and update results at a regular interval.
// Takes a context to store and synchronize with the calling thread
func (td *OsmTestData) AsynchronousHTTPRunner(context HTTPAsync) {
	context.WaitGroup.Add(1)
	defer context.WaitGroup.Done()

	for !*context.StopSignal {
		result := td.HTTPRequest(context.RequestData)

		context.ResponseDataStore.withLock(func() {
			context.ResponseDataStore.lastResult = result
		})

		time.Sleep(context.SleepTime)
	}
}

// PrettyPrintHTTPResults prints a line of results. Takes Title/App/NS name, list of pod names and list of results
func (td *OsmTestData) PrettyPrintHTTPResults(appName string, podNames []string, asyncRes []*HTTPAsyncResults) {
	Expect(len(podNames)).To(Equal(len(asyncRes)))

	strLine := fmt.Sprintf("%s - ", color.CyanString(appName))
	for idx := range podNames {
		strLine += fmt.Sprintf("%s: %s -", podNames[idx], getColoredStatusCode(asyncRes[idx]))
	}
	td.T.Log(strLine)
}

func getColoredStatusCode(res *HTTPAsyncResults) string {
	var coloredStatus string
	res.withLock(func() {
		if res.lastResult.Err != nil {
			coloredStatus = color.RedString("ERR")
		} else if res.lastResult.StatusCode != 200 {
			coloredStatus = color.YellowString("%d ", res.lastResult.StatusCode)
		} else {
			coloredStatus = color.HiGreenString("%d ", res.lastResult.StatusCode)
		}
	})
	return coloredStatus
}
