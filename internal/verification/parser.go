package verification

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

type CaseResult struct {
	Identity string `json:"identity"`
	Status   string `json:"status"`
}

type ParsedOutput struct {
	SchemaVersion string          `json:"schema_version"`
	Kind          ParserKind      `json:"kind"`
	Cases         []CaseResult    `json:"cases"`
	Structured    json.RawMessage `json:"structured"`
}

func parseOutput(gate Gate, stdout []byte, exitCode int) (json.RawMessage, []string, error) {
	parsed := ParsedOutput{SchemaVersion: "revolvr-parsed-verification-output-v1", Kind: gate.Parser.Kind, Structured: json.RawMessage(`{}`)}
	var err error
	switch gate.Parser.Kind {
	case ParserNone:
		status := "passed"
		if exitCode != 0 {
			status = "failed"
		}
		parsed.Cases = []CaseResult{{Identity: "gate:" + gate.ID, Status: status}}
	case ParserGoTestJSON:
		parsed.Cases, parsed.Structured, err = parseGoTestJSON(stdout)
	case ParserJSON:
		parsed.Cases, parsed.Structured, err = parseGenericJSON(stdout)
	case ParserJUnitXML:
		parsed.Cases, parsed.Structured, err = parseJUnitXML(stdout)
	default:
		err = fmt.Errorf("unsupported parser %q", gate.Parser.Kind)
	}
	if err != nil {
		return nil, nil, err
	}
	if len(parsed.Cases) == 0 {
		status := "passed"
		if exitCode != 0 {
			status = "failed"
		}
		parsed.Cases = []CaseResult{{Identity: "gate:" + gate.ID, Status: status}}
	}
	sort.Slice(parsed.Cases, func(i, j int) bool {
		if parsed.Cases[i].Identity == parsed.Cases[j].Identity {
			return parsed.Cases[i].Status < parsed.Cases[j].Status
		}
		return parsed.Cases[i].Identity < parsed.Cases[j].Identity
	})
	failures := make([]string, 0)
	for _, result := range parsed.Cases {
		if result.Status == "failed" {
			failures = append(failures, result.Identity)
		}
	}
	raw, err := json.Marshal(parsed)
	return raw, failures, err
}

func parseGoTestJSON(raw []byte) ([]CaseResult, json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil, io.ErrUnexpectedEOF
	}
	type event struct {
		Action  string `json:"Action"`
		Package string `json:"Package"`
		Test    string `json:"Test"`
	}
	var events []json.RawMessage
	var cases []CaseResult
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64<<10), int(MaximumCapturedStreamBytes))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var value event
		var structured json.RawMessage
		if json.Unmarshal(line, &value) != nil || json.Unmarshal(line, &structured) != nil {
			return nil, nil, fmt.Errorf("malformed go test JSON")
		}
		events = append(events, append(json.RawMessage(nil), structured...))
		if value.Action != "pass" && value.Action != "fail" {
			continue
		}
		name := value.Package
		if value.Test != "" {
			name += "/" + value.Test
		}
		if name == "" {
			continue
		}
		status := "passed"
		if value.Action == "fail" {
			status = "failed"
		}
		cases = append(cases, CaseResult{Identity: "go-test:" + name, Status: status})
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	structured, err := json.Marshal(events)
	return cases, structured, err
}

func parseGenericJSON(raw []byte) ([]CaseResult, json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, nil, fmt.Errorf("malformed structured JSON")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, nil, fmt.Errorf("structured JSON contains trailing data")
	}
	structured, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	var cases []CaseResult
	collectJSONCases(value, &cases)
	return cases, structured, nil
}

func collectJSONCases(value any, cases *[]CaseResult) {
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			collectJSONCases(child, cases)
		}
	case map[string]any:
		status, _ := typed["status"].(string)
		if status == "" {
			status, _ = typed["outcome"].(string)
		}
		name, _ := typed["name"].(string)
		if name == "" {
			name, _ = typed["id"].(string)
		}
		switch strings.ToLower(status) {
		case "pass", "passed", "success":
			if name != "" {
				*cases = append(*cases, CaseResult{Identity: "json:" + name, Status: "passed"})
			}
		case "fail", "failed", "failure", "error":
			if name != "" {
				*cases = append(*cases, CaseResult{Identity: "json:" + name, Status: "failed"})
			}
		}
		for _, child := range typed {
			collectJSONCases(child, cases)
		}
	}
}

type junitSuites struct {
	Suites []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name  string      `xml:"name,attr"`
	Cases []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string    `xml:"name,attr"`
	Classname string    `xml:"classname,attr"`
	Failure   *struct{} `xml:"failure"`
	Error     *struct{} `xml:"error"`
	Skipped   *struct{} `xml:"skipped"`
}

func parseJUnitXML(raw []byte) ([]CaseResult, json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil, io.ErrUnexpectedEOF
	}
	var root struct {
		XMLName xml.Name
		Suites  []junitSuite `xml:"testsuite"`
		Cases   []junitCase  `xml:"testcase"`
	}
	if err := xml.Unmarshal(raw, &root); err != nil {
		return nil, nil, fmt.Errorf("malformed JUnit XML: %w", err)
	}
	suites := root.Suites
	if root.XMLName.Local == "testsuite" {
		suites = []junitSuite{{Cases: root.Cases}}
	}
	var cases []CaseResult
	for _, suite := range suites {
		for _, test := range suite.Cases {
			if test.Skipped != nil {
				continue
			}
			name := strings.Trim(strings.Join([]string{test.Classname, test.Name}, "/"), "/")
			if name == "" {
				continue
			}
			status := "passed"
			if test.Failure != nil || test.Error != nil {
				status = "failed"
			}
			cases = append(cases, CaseResult{Identity: "junit:" + name, Status: status})
		}
	}
	structured, err := json.Marshal(cases)
	return cases, structured, err
}

func ClassifyDifferential(baseline, candidate []PersistedCheck) Differential {
	baseFailures, baseStates := classifiedCases(baseline)
	candidateFailures, candidateStates := classifiedCases(candidate)
	flaky := map[string]bool{}
	for identity, states := range baseStates {
		if len(states) > 1 {
			flaky[identity] = true
		}
	}
	for identity, states := range candidateStates {
		if len(states) > 1 {
			flaky[identity] = true
		}
	}
	result := Differential{}
	for identity := range candidateFailures {
		switch {
		case flaky[identity]:
		case baseFailures[identity]:
			result.Unchanged = append(result.Unchanged, identity)
		default:
			result.New = append(result.New, identity)
		}
	}
	for identity := range baseFailures {
		if flaky[identity] || candidateFailures[identity] {
			continue
		}
		result.Resolved = append(result.Resolved, identity)
	}
	for identity := range flaky {
		result.Flaky = append(result.Flaky, identity)
	}
	for _, list := range []*[]string{&result.New, &result.Resolved, &result.Unchanged, &result.Flaky} {
		sort.Strings(*list)
	}
	return result
}

func classifiedCases(checks []PersistedCheck) (map[string]bool, map[string]map[string]bool) {
	failures := map[string]bool{}
	states := map[string]map[string]bool{}
	for _, check := range checks {
		var parsed ParsedOutput
		if json.Unmarshal(check.ParsedResult, &parsed) == nil {
			for _, result := range parsed.Cases {
				if states[result.Identity] == nil {
					states[result.Identity] = map[string]bool{}
				}
				states[result.Identity][result.Status] = true
				if result.Status == "failed" {
					failures[result.Identity] = true
				}
			}
		}
		for _, identity := range check.FailureSignatures {
			failures[identity] = true
			if states[identity] == nil {
				states[identity] = map[string]bool{}
			}
			states[identity]["failed"] = true
		}
	}
	return failures, states
}
