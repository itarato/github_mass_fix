// The script requires your GitHub token.
// It fetches the teams for a given organisation.
// Then goes through one team's repos.

// Call: go run main.go - dry run, no commits
// Call: go run main.go commit

// Config file format:
// {
//   "personalToken": "TOKEN_FROM_GITHUB",
//   "orgName": "SELECTED_ORG_NAME",
//   "teamName": "SELECTED_TEAM_NAME"
// }

package main

import (
	"encoding/json"
	"errors"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
)

var (
	repoPageLimit int    = 30
	tempCloneName string = "temp_clone"
	flagCommit    bool   = false
)

type Config struct {
	PersonalToken string `json:"personalToken"`
	OrgName       string `json:"orgName"`
	TeamName      string `json:"teamName"`
	TempDir       string `json:"tempDir"`
	GitCMD        string `json:"gitCmd"`
	GrepCMD       string `json:"grepCmd"`
	SedCMD        string `json:"sedCmd"`
	RepoPattern   string `json:"repoPattern"`
}

type GithubTokenSource struct {
	PersonalToken string
}

func (gts *GithubTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: gts.PersonalToken}, nil
}

func getConfig() (*Config, error) {
	var config Config
	b, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Println(err)
		return nil, errors.New("No config file")
	}

	if err = json.Unmarshal(b, &config); err != nil {
		log.Println(err)
		return nil, errors.New("Invalid config format")
	}

	return &config, nil
}

func fetchRepos(config *Config, client *github.Client) []github.Repository {
	log.Println("Fetch organization teams")
	repos := make([]github.Repository, 0, 1000)
	teams, _, err := client.Organizations.ListTeams(config.OrgName, nil)
	if err != nil {
		log.Println(err)
	}
	log.Println("Found", len(teams), "teams")

	var team_id int
	for _, team := range teams {
		if *team.Name == config.TeamName {
			team_id = *team.ID
			break
		}
	}
	log.Println("Inspected team found, ID:", team_id)

	log.Println("Fetch repositories")
	for page := 0; ; page++ {
		opt := &github.ListOptions{PerPage: repoPageLimit, Page: page}
		repos_paged, _, err := client.Organizations.ListTeamRepos(team_id, opt)
		if err != nil {
			log.Println(err)
			break
		}

		// No more repositories
		if len(repos_paged) == 0 {
			break
		}

		for _, repo := range repos_paged {
			repos = append(repos, repo)
		}
	}

	return repos
}

func execCmdWithOutput(arg0 string, args ...string) (string, error) {
	cmd := exec.Command(arg0, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	if cmd_err := cmd.Start(); cmd_err != nil {
		return "", cmd_err
	}

	output, err_out := ioutil.ReadAll(stdout)
	if err_out != nil {
		return "", err_out
	}

	return string(output), nil
}

func handleRepo(config *Config, sshURL string) {
	log.Println("Repo clone", sshURL)

	syscall.Chdir(config.TempDir)
	os.RemoveAll("./" + tempCloneName)

	_, err_clone := exec.Command(config.GitCMD, "clone", "--branch", "7.x-1.x-stable", sshURL, tempCloneName).Output()
	if err_clone != nil {
		log.Println("Repo cannot be cloned")
		return
	}

	out_grep, _ := execCmdWithOutput(config.GrepCMD, "-rl", "url.full.pdf", tempCloneName)
	out_grep_trimmed := strings.Trim(out_grep, "\n\r\t ")
	if out_grep_trimmed == "" {
		log.Println("No match")
		return
	}

	files := strings.Split(out_grep_trimmed, "\n")
	for _, fileName := range files {
		handleFile(config, fileName)
	}

	syscall.Chdir("./" + tempCloneName)
	diff, _ := execCmdWithOutput(config.GitCMD, "diff")

	if flagCommit {
		execCmdWithOutput(config.GitCMD, "commit", "-a", "-m", "\"Fixed by script.\"", ".")
	}

	syscall.Chdir(config.TempDir)
	log.Println(diff)
}

func handleFile(config *Config, fileName string) {
	log.Println("Edit", fileName)
	new_content, _ := execCmdWithOutput(config.SedCMD, "-e", "s/url\\.full\\.pdf/url_full_pdf/g", fileName)
	file, err := os.OpenFile(fileName, os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal("File cannot opened", fileName)
	}

	file.WriteString(new_content)
	file.Close()
}

func execute(config *Config) {
	c := oauth2.NewClient(oauth2.NoContext, &GithubTokenSource{config.PersonalToken})
	client := github.NewClient(c)

	repos := fetchRepos(config, client)
	log.Println("Found", len(repos), "repositories")

	for _, repo := range repos {
		isMatch, _ := regexp.MatchString(config.RepoPattern, *repo.SSHURL)
		if isMatch {
			handleRepo(config, *repo.SSHURL)
		} else {
			log.Println("Ignored:", *repo.SSHURL)
		}
	}
}

func main() {
	log.Println("Repository bundle update script - START")

	flagCommit = len(os.Args) == 2 && os.Args[1] == "commit"

	config, err := getConfig()
	if err != nil {
		log.Fatalln("Configuration not found")
	}

	execute(config)

	log.Println("END")
}
