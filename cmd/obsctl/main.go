package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/onboard"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "status":
		err = cmdStatus()
	case "services":
		err = cmdServices(os.Args[2:])
	case "service":
		err = cmdService(os.Args[2:])
	case "incident":
		err = cmdIncident(os.Args[2:])
	case "slo":
		err = cmdSLO(os.Args[2:])
	case "demo":
		err = cmdDemo(os.Args[2:])
	case "runbook":
		err = cmdGet("/api/v1/runbooks")
	case "deployments":
		err = cmdGet("/api/v1/deployments")
	case "anomalies":
		err = cmdGet("/api/v1/anomalies")
	case "help", "-h", "--help":
		usage()
		return
	default:
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `obsctl is the operator CLI for the Cloud Observability & AIOps Platform.

Usage:
  obsctl status
  obsctl services list
  obsctl service onboard --name orders --team checkout --environment local
  obsctl incident list
  obsctl incident show INC-2026-0001
  obsctl incident analyze INC-2026-0001
  obsctl incident postmortem INC-2026-0001
  obsctl incident transition INC-2026-0001 investigating
  obsctl slo status
  obsctl demo fault enable payment-latency
  obsctl demo fault disable payment-latency
  obsctl demo fault list
  obsctl runbook list
  obsctl deployments list

Environment:
  OBSCTL_API           Platform base URL (default http://localhost:8080)
  PLATFORM_API_TOKEN   Bearer token when the API requires one
  OBSCTL_ACTOR         Actor recorded on mutations (default $USER or obsctl)
`)
}

func cmdStatus() error {
	return cmdGet("/health")
}

func cmdServices(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		var body struct {
			Services []map[string]any `json:"services"`
		}
		if err := getJSON("/api/v1/services", &body); err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTEAM\tCRITICALITY\tENVIRONMENT")
		for _, s := range body.Services {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s["name"], s["team"], s["criticality"], s["environment"])
		}
		return w.Flush()
	}
	return fmt.Errorf("unknown services command")
}

func cmdService(args []string) error {
	if len(args) < 1 || args[0] != "onboard" {
		return fmt.Errorf("usage: obsctl service onboard --name NAME --team TEAM --environment ENV")
	}
	opts := onboard.Options{Repo: ".", Environment: "local", Criticality: "medium"}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--name":
			i++
			opts.Name = args[i]
		case "--team":
			i++
			opts.Team = args[i]
		case "--environment", "--env":
			i++
			opts.Environment = args[i]
		case "--owner":
			i++
			opts.Owner = args[i]
		case "--criticality":
			i++
			opts.Criticality = args[i]
		case "--repo":
			i++
			opts.Repo = args[i]
		default:
			return fmt.Errorf("unknown flag %s", args[i])
		}
	}
	paths, err := onboard.Service(opts)
	if err != nil {
		return err
	}
	fmt.Println("wrote:")
	for _, p := range paths {
		fmt.Println(" ", p)
	}
	return nil
}

func cmdIncident(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		var body struct {
			Incidents []map[string]any `json:"incidents"`
		}
		if err := getJSON("/api/v1/incidents", &body); err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSEV\tSTATUS\tSERVICE\tTITLE")
		for _, inc := range body.Incidents {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", inc["incident_id"], inc["severity"], inc["status"], inc["service"], inc["title"])
		}
		return w.Flush()
	}
	if len(args) < 2 {
		return fmt.Errorf("incident id required")
	}
	id := args[1]
	switch args[0] {
	case "show":
		return cmdGet("/api/v1/incidents/" + id)
	case "analyze":
		return cmdPost("/api/v1/incidents/"+id+"/analyze", nil)
	case "postmortem":
		return cmdGetRaw("/api/v1/incidents/" + id + "/postmortem")
	case "transition":
		if len(args) < 3 {
			return fmt.Errorf("usage: obsctl incident transition ID STATUS")
		}
		return cmdPost("/api/v1/incidents/"+id+"/transition", map[string]string{"status": args[2], "reason": "operator transition"})
	default:
		return fmt.Errorf("unknown incident command")
	}
}

func cmdSLO(args []string) error {
	if len(args) == 0 || args[0] == "status" {
		return cmdGet("/api/v1/slos")
	}
	return fmt.Errorf("unknown slo command")
}

func cmdDemo(args []string) error {
	if len(args) < 2 || args[0] != "fault" {
		return fmt.Errorf("usage: obsctl demo fault enable|disable|list NAME")
	}
	switch args[1] {
	case "list":
		return cmdGet("/api/v1/demo/faults")
	case "enable":
		if len(args) < 3 {
			return fmt.Errorf("fault name required")
		}
		return cmdPost("/api/v1/demo/faults", map[string]string{"name": args[2]})
	case "disable":
		if len(args) < 3 {
			return fmt.Errorf("fault name required")
		}
		return cmdDelete("/api/v1/demo/faults/" + args[2])
	default:
		return fmt.Errorf("unknown fault command")
	}
}

func cmdGet(path string) error {
	var v any
	if err := getJSON(path, &v); err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func cmdGetRaw(path string) error {
	resp, err := do(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, bytes.TrimSpace(body))
	}
	fmt.Println(string(body))
	return nil
}

func cmdPost(path string, payload any) error {
	var buf io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		buf = bytes.NewReader(b)
	}
	resp, err := do(http.MethodPost, path, buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResp(resp)
}

func cmdDelete(path string) error {
	resp, err := do(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResp(resp)
}

func getJSON(path string, dest any) error {
	resp, err := do(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, bytes.TrimSpace(body))
	}
	return json.Unmarshal(body, dest)
}

func printResp(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, bytes.TrimSpace(body))
	}
	var v any
	if json.Unmarshal(body, &v) == nil {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	fmt.Println(string(body))
	return nil
}

func do(method, path string, body io.Reader) (*http.Response, error) {
	base := os.Getenv("OBSCTL_API")
	if base == "" {
		base = "http://localhost:8080"
	}
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := os.Getenv("PLATFORM_API_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	actor := os.Getenv("OBSCTL_ACTOR")
	if actor == "" {
		actor = os.Getenv("USER")
	}
	if actor == "" {
		actor = "obsctl"
	}
	req.Header.Set("X-Actor", actor)
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}
