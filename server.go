package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

// Route is ...
type Route struct {
	Method string            `yaml:"method"`
	Path   string            `yaml:"path"`
	Script string            `yaml:"script"`
	Binary string            `yaml:"binary"`
	Params map[string]string `yaml:"-"`
}

// Routes is a list of Routes.
type Routes struct {
	Routes []Route `yaml:"routes"`
}

type byPathLength []Route

func (b byPathLength) Len() int           { return len(b) }
func (b byPathLength) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byPathLength) Less(i, j int) bool { return len(b[i].Path) > len(b[j].Path) }

const timeout = 500 * time.Millisecond

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v\n", err)
	}
}

func run() error {
	cgiScriptsDir := os.Getenv("CGI_DIR")
	if cgiScriptsDir == "" {
		return fmt.Errorf("CGI_DIR environment variable not set")
	}

	routes, err := readAndUnmarshalRoutes(cgiScriptsDir + "/routes.yaml")
	if err != nil {
		return fmt.Errorf("read and unmarshal routes: %w", err)
	}

	// Build each Roc script into an executable
	for _, route := range routes.Routes {
		scriptPath := path.Join(cgiScriptsDir, route.Script)
		binaryPath := path.Join(cgiScriptsDir, route.Binary)

		cmd := exec.Command("roc", "build", "--optimize", scriptPath)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("unable to build %s: %w", scriptPath, err)
		}

		// Check that the executable exists with the expected name
		if _, err := os.Stat(binaryPath); errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("expected binary %s does not exist", binaryPath)
		}
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if err := handleHTTPRequest(w, r, routes, cgiScriptsDir); err != nil {
			statusCode := 500
			var errStatusCode statusCodeError
			if errors.As(err, &errStatusCode) {
				statusCode = int(errStatusCode)
			}

			log.Printf("Error: %v", err)
			w.WriteHeader(statusCode)
			return
		}

		elapsed := time.Since(start)

		log.Printf("%s %s %d ms\n", r.Method, r.RequestURI, elapsed.Milliseconds())
	})

	log.Printf("INFO: Listening on port 8080\n")
	if err := http.ListenAndServe(":8080", nil); err != http.ErrServerClosed {
		return fmt.Errorf("HTTP Server failed: %w", err)
	}

	return nil
}

func readAndUnmarshalRoutes(routeFile string) (Routes, error) {
	data, err := os.ReadFile(routeFile)
	if err != nil {
		return Routes{}, fmt.Errorf("unable to read file: %w", err)
	}

	var routes Routes
	err = yaml.Unmarshal(data, &routes)
	if err != nil {
		return Routes{}, fmt.Errorf("unable to decode YAML: %w", err)
	}

	sort.Sort(byPathLength(routes.Routes))

	return routes, nil
}

func findRoute(r *http.Request, routes Routes) (*Route, bool) {
	for _, route := range routes.Routes {
		params := parseURLParameters(r.URL.Path, route.Path)
		if params != nil && r.Method == route.Method {
			route.Params = params
			return &route, true
		}
	}

	return nil, false
}

func handleHTTPRequest(w http.ResponseWriter, r *http.Request, routes Routes, cgiScriptsDir string) error {
	route, ok := findRoute(r, routes)
	if !ok {
		return errors.Join(
			fmt.Errorf("no script found for route"),
			statusCodeError(http.StatusBadRequest),
		)
	}

	if err := executeScript(w, r, *route, path.Join(cgiScriptsDir, route.Binary)); err != nil {
		return fmt.Errorf("execute script: %w", err)
	}

	return nil
}

type statusCodeError int

func (err statusCodeError) Error() string {
	return fmt.Sprintf("status code: %d", err)
}

func executeScript(w http.ResponseWriter, r *http.Request, route Route, binaryPath string) error {
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Stdin = r.Body
	cmd.Stdout = w
	cmd.Env = []string{
		"REQUEST_METHOD=" + r.Method,
		"REQUEST_URI=" + r.RequestURI,
		"QUERY_STRING=" + r.URL.RawQuery,
		"CONTENT_LENGTH=" + r.Header.Get("Content-Length"),
		"CONTENT_TYPE=" + r.Header.Get("Content-Type"),
		"REMOTE_ADDR=" + r.RemoteAddr,
		"SERVER_PROTOCOL=" + r.Proto,
		"SERVER_SOFTWARE=roc-cgi",
		"SCRIPT_NAME=" + route.Script,
	}

	for key, value := range route.Params {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	err := cmd.Run()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.Join(
				fmt.Errorf("time out. Cancelling"),
				statusCodeError(http.StatusRequestTimeout),
			)
		}

		return errors.Join(
			fmt.Errorf("unable to run command: %w", err),
			statusCodeError(500),
		)
	}

	return nil
}

func parseURLParameters(url, pattern string) map[string]string {
	urlSegments := strings.Split(url, "/")
	patternSegments := strings.Split(pattern, "/")

	if len(urlSegments) != len(patternSegments) {
		return nil
	}

	parameters := make(map[string]string, len(patternSegments))

	for i, segment := range patternSegments {
		if len(segment) > 2 && segment[0] == '{' && segment[len(segment)-1] == '}' {
			paramName := segment[1 : len(segment)-1]
			parameters[paramName] = urlSegments[i]
			continue
		}

		if segment != urlSegments[i] {
			return nil
		}
	}

	return parameters
}
