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

type Route struct {
	Method string            `yaml:"method"`
	Path   string            `yaml:"path"`
	Script string            `yaml:"script"`
	Binary string            `yaml:"binary"`
	Params map[string]string `yaml:"-"`
}

type Routes struct {
	Routes []Route `yaml:"routes"`
}

type ByPathLength []Route

func (b ByPathLength) Len() int           { return len(b) }
func (b ByPathLength) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByPathLength) Less(i, j int) bool { return len(b[i].Path) > len(b[j].Path) }

const TIMEOUT_MILLISECONDS = 500

var ErrInternalServerGeneric = errors.New("oops, something went wrong")

func main() {

	cgi_scripts_dir := os.Getenv("CGI_DIR")
	if cgi_scripts_dir == "" {
		log.Fatalf("ERROR: CGI_DIR environment variable not set\n")
	}

	routes := readAndUnmarshalRoutes(cgi_scripts_dir + "/routes.yaml")

	// Build each Roc script into an executable
	for _, route := range routes.Routes {

		script_path := path.Join(cgi_scripts_dir, route.Script)
		binary_path := path.Join(cgi_scripts_dir, route.Binary)

		cmd := exec.Command("roc", "build", script_path)
		// --optimize runs slow on Roc PG for now
		// cmd := exec.Command("roc", "build", "--optimize", script_path)
		err := cmd.Run()
		if err != nil {
			log.Fatalf("ERROR: Unable to build %s: %s", script_path, err.Error())
		}

		// Check that the executable exists with the expected name
		if _, err := os.Stat(binary_path); os.IsNotExist(err) {
			log.Fatalf("ERROR: Expected binary %s does not exist", binary_path)
		}

	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		start := time.Now()

		handleHTTPRequest(w, r, routes, cgi_scripts_dir)

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

	sort.Sort(ByPathLength(routes.Routes))

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

func handleHTTPRequest(w http.ResponseWriter, r *http.Request, routes Routes, cgi_scripts_dir string) {
	route := findRoute(r, routes)
	if route == nil {
		http.Error(w, "no script found for route", http.StatusBadRequest)
		return
	}

	statusCode, err := executeScript(w, r, *route, path.Join(cgi_scripts_dir, route.Binary))
	if err != nil {
		http.Error(w, err.Error(), statusCode)
	}
}

func executeScript(w http.ResponseWriter, r *http.Request, route Route, binary_path string) (int, error) {

	ctx, cancel := context.WithTimeout(context.Background(), TIMEOUT_MILLISECONDS*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary_path)
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

	done := make(chan error)
	go func() {
		err := cmd.Run()
		if err != nil {
			done <- fmt.Errorf("ERROR: Unable to run command: %s", err.Error())
		} else {
			done <- nil
		}
	}()

	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("Timed out. Cancelling...")
			cmd.Process.Kill()
		}
	case err := <-done:
		if err != nil {
			log.Printf("ERROR: Command failed: %s", err)
			return http.StatusRequestTimeout, fmt.Errorf("ERROR: Command failed: %s", err.Error())
		}
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
