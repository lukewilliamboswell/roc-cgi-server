package main

import (
	"context"
	"errors"
	"fmt"
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

const timeoutMilliseconds = 500

var errInternalServerGeneric = errors.New("oops, something went wrong")

func main() {
	cgiScriptsDir := os.Getenv("CGI_DIR")
	if cgiScriptsDir == "" {
		log.Fatalf("ERROR: CGI_DIR environment variable not set\n")
	}

	routes := readAndUnmarshalRoutes(cgiScriptsDir + "/routes.yaml")

	// Build each Roc script into an executable
	for _, route := range routes.Routes {

		scriptPath := path.Join(cgiScriptsDir, route.Script)
		binaryPath := path.Join(cgiScriptsDir, route.Binary)

		cmd := exec.Command("roc", "build", scriptPath)
		// --optimize runs slow on Roc PG for now
		// cmd := exec.Command("roc", "build", "--optimize", script_path)
		err := cmd.Run()
		if err != nil {
			log.Fatalf("ERROR: Unable to build %s: %s", scriptPath, err.Error())
		}

		// Check that the executable exists with the expected name
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			log.Fatalf("ERROR: Expected binary %s does not exist", binaryPath)
		}

	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		handleHTTPRequest(w, r, routes, cgiScriptsDir)

		elapsed := time.Since(start)

		log.Printf("%s %s %d ms\n", r.Method, r.RequestURI, elapsed.Milliseconds())
	})

	log.Printf("INFO: Listening on port 8080\n")
	http.ListenAndServe(":8080", nil)
}

func readAndUnmarshalRoutes(routeFile string) Routes {
	data, err := os.ReadFile(routeFile)
	if err != nil {
		log.Fatalf("ERROR: Unable to read file: %s", err.Error())
	}

	var routes Routes
	err = yaml.Unmarshal(data, &routes)
	if err != nil {
		log.Fatalf("ERROR: Unable to decode YAML: %s", err.Error())
	}

	sort.Sort(byPathLength(routes.Routes))

	return routes
}

func findRoute(r *http.Request, routes Routes) *Route {
	for _, route := range routes.Routes {
		params := parseURLParameters(r.URL.Path, route.Path)
		if params != nil && r.Method == route.Method {
			route.Params = params
			return &route
		}
	}

	return nil
}

func handleHTTPRequest(w http.ResponseWriter, r *http.Request, routes Routes, cgiScriptsDir string) {
	route := findRoute(r, routes)
	if route == nil {
		http.Error(w, "no script found for route", http.StatusBadRequest)
		return
	}

	statusCode, err := executeScript(w, r, *route, path.Join(cgiScriptsDir, route.Binary))
	if err != nil {
		http.Error(w, err.Error(), statusCode)
	}
}

func executeScript(w http.ResponseWriter, r *http.Request, route Route, binaryPath string) (int, error) {
	ctx, cancel := context.WithTimeout(r.Context(), timeoutMilliseconds*time.Millisecond)
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
			log.Printf("Timed out. Cancelling...")
		}
		return 500, fmt.Errorf("ERROR: Unable to run command: %w", err)
	}

	return http.StatusOK, nil
}

func parseURLParameters(url, pattern string) map[string]string {
	urlSegments := strings.Split(url, "/")
	patternSegments := strings.Split(pattern, "/")

	if len(urlSegments) != len(patternSegments) {
		return nil
	}

	parameters := make(map[string]string)

	for i, segment := range patternSegments {
		if len(segment) > 2 && segment[0] == '{' && segment[len(segment)-1] == '}' {
			paramName := segment[1 : len(segment)-1]
			parameters[paramName] = urlSegments[i]
		} else if segment != urlSegments[i] {
			return nil
		}
	}

	return parameters
}
