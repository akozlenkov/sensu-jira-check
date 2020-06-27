package main

import (
	"fmt"
	"github.com/sensu/sensu-go/types"
	"gopkg.in/andygrunwald/go-jira.v1"
	"os"
	"text/template"
	"github.com/sensu-community/sensu-plugin-sdk/sensu"
	corev2 "github.com/sensu/sensu-go/api/core/v2"
	"github.com/prologic/bitcask"
)

type HandlerConfig struct {
	sensu.PluginConfig
	jiraUrl             string
	jiraUser            string
	jiraPassword        string
	jiraQuery	        string
	outputTemplate      string
	knownIssues			string
}

const (
	outputTemplate = `{{ if ne (len .) 0 }}
Found {{ (len .) }} new tickets
{{range . -}}
{{.Key}} {{.Fields.Summary}}
{{ "" }}
{{- end}}
{{end}}`
	knownIssues = "/tmp/sensu-jira-check"
)

var (
	config = HandlerConfig{
		PluginConfig: sensu.PluginConfig{
			Name:     "sensu-jira-check",
			Short:    "The Sensu Go jira check",
			Keyspace: "github.com/akozlenkov/sensu-jira-check",
		},
	}

	configOptions = []*sensu.PluginConfigOption{
		{
			Path:     "jira-url",
			Env:      "JIRA_URL",
			Argument: "jira-url",
			Usage:    "The jira URL",
			Value:    &config.jiraUrl,
		},
		{
			Path:     "jira-user",
			Env:      "JIRA_USER",
			Argument: "jira-user",
			Usage:    "The jira user",
			Value:    &config.jiraUser,
		},
		{
			Path:     "jira-password",
			Env:      "JIRA_PASSWORD",
			Argument: "jira-password",
			Usage:    "The jira password",
			Value:    &config.jiraPassword,
		},
		{
			Path:     "jira-query",
			Argument: "jira-query",
			Usage:    "The jira query",
			Value:    &config.jiraQuery,
		},
		{
			Path:     "output-template",
			Argument: "output-template",
			Default:  outputTemplate,
			Usage:    "The output template",
			Value:    &config.outputTemplate,
		},
		{
			Path:     "known-issues",
			Argument: "known-issues",
			Default:  knownIssues,
			Usage:    "The known issues file path",
			Value:    &config.knownIssues,
		},
	}
)

func checkArgs(_ *corev2.Event) (int, error) {
	if jiraUrl := os.Getenv("JIRA_URL"); jiraUrl != "" {
		config.jiraUrl = jiraUrl
	}

	if jiraUser := os.Getenv("JIRA_USER"); jiraUser != "" {
		config.jiraUser = jiraUser
	}

	if jiraPassword := os.Getenv("JIRA_PASSWORD"); jiraPassword != "" {
		config.jiraPassword = jiraPassword
	}

	if len(config.jiraUrl) == 0 {
		return 3, fmt.Errorf("--jira-url or JIRA_URL environment variable is required")
	}

	if len(config.jiraUser) == 0 {
		return 3, fmt.Errorf("--jira-user or JIRA_USER environment variable is required")
	}

	if len(config.jiraPassword) == 0 {
		return 3, fmt.Errorf("--jira-password or JIRA_PASSWORD environment variable is required")
	}
	return 0, nil
}

func checkFunc(event *types.Event) (int, error) {
	db, err := bitcask.Open(config.knownIssues)
	if err != nil {
		return 2, err
	}
	defer db.Close()

	tp := jira.BasicAuthTransport{
		Username: config.jiraUser,
		Password: config.jiraPassword,
	}

	client, err := jira.NewClient(tp.Client(), config.jiraUrl)
	if err != nil {
		return 2, err
	}

	var issues []jira.Issue

	if err = client.Issue.SearchPages(config.jiraQuery, nil, func(issue jira.Issue) error {
		issues = append(issues, issue)
		return err
	}); err != nil {
		return 2, err
	}

	var newIssues []jira.Issue
	for _, issue := range issues {
		if !db.Has([]byte(issue.ID)) {
			newIssues = append(newIssues, issue)
			if err := db.Put([]byte(issue.ID), []byte(issue.Fields.Summary)); err != nil {
				return 2, err
			}
		}
	}

	tmpl, err := template.New("output").Parse(config.outputTemplate)
	if err != nil {
		return 2, err
	}

	if err := tmpl.Execute(os.Stdout, newIssues); err != nil {
		return 2, err
	}

	return 0, nil
}

func main()  {
	check := sensu.NewGoCheck(&config.PluginConfig, configOptions, checkArgs, checkFunc, false)
	check.Execute()
}
