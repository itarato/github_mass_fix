package main

import (
	"encoding/json"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var (
	repoPageLimit       int        = 30            // Limit of a simgle REST get
	tempCloneNamePrefix string     = "temp_clone_" // Dir prefix for cloned repos
	flagCommit          bool       = false         // Flag - commit or not the result
	repoCounter         int        = 0             // Incremental counter to differenciate repos
	mutexInRepoOp       sync.Mutex                 // Mutex to keep in-git-dir operations safe
)

// Configuration container loaded from JSON
type Config struct {
	PersonalToken string `json:"personalToken"`
	OrgName       string `json:"orgName"`
	TeamName      string `json:"teamName"`
	TempDir       string `json:"tempDir"`
	GitCMD        string `json:"gitCmd"`
	GrepCMD       string `json:"grepCmd"`
	SedCMD        string `json:"sedCmd"`
	RepoPattern   string `json:"repoPattern"`
	ReplaceFrom   string `json:"replaceFrom"`
	ReplaceTo     string `json:"replaceTo"`
	Branch        string `json:"branch"`
	CommitMessage string `json:"commitMessage"`
}

// Token provider
type GithubTokenSource struct {
	PersonalToken string
}

// Return the wrapped token for GitHub authentication client
func (gts *GithubTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: gts.PersonalToken}, nil
}

// Load configuration
func getConfig() (*Config, error) {
	var config Config
	b, err := ioutil.ReadFile("config.json")
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(b, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// Load repositories using the configuration
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
	for page := 1; ; page++ {
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

// Execute a shell command and returns the output
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

// Create a temp name for the cloned repo folder
func getTempRepoName(sshURL string) string {
	rx, rx_err := regexp.Compile("^.*\\/([^\\/]{1,}).git$")
	if rx_err != nil {
		log.Fatal(rx_err)
	}
	repoCounter++
	return tempCloneNamePrefix + rx.FindStringSubmatch(sshURL)[1] + "_" + strconv.Itoa(repoCounter)
}

// Clones a repository and attempt to change files
func handleRepo(config *Config, sshURL string) {
	log.Println("Repo clone", sshURL)

	tempCloneName := getTempRepoName(sshURL)

	syscall.Chdir(config.TempDir)
	os.RemoveAll("./" + tempCloneName)
	defer func() {
		syscall.Chdir(config.TempDir)
		os.RemoveAll("./" + tempCloneName)
	}()

	_, err_clone := exec.Command(config.GitCMD, "clone", "--branch", config.Branch, sshURL, tempCloneName).Output()
	if err_clone != nil {
		log.Println("Repo cannot be cloned", sshURL, err_clone)
		return
	}

	out_grep, err_grep := execCmdWithOutput(config.GrepCMD, "-rl", "--exclude-dir", ".git", config.ReplaceFrom, tempCloneName)
	if err_grep != nil {
		log.Panic(err_grep)
		return
	}

	out_grep_trimmed := strings.Trim(out_grep, "\n\r\t ")
	if out_grep_trimmed == "" {
		log.Println("No match")
		return
	}

	files := strings.Split(out_grep_trimmed, "\n")
	for _, fileName := range files {
		handleFile(config, fileName)
	}

	// Make git operations safe - they have to be called from the directory
	mutexInRepoOp.Lock()
	syscall.Chdir("./" + tempCloneName)
	diff, err_diff := execCmdWithOutput(config.GitCMD, "diff")
	if err_diff != nil {
		log.Panic(err_diff)
		return
	}
	log.Println(diff)

	if flagCommit {
		log.Println("Committing changes")
		_, err_commit := execCmdWithOutput(config.GitCMD, "commit", "-a", "-m", config.CommitMessage)
		if err_commit != nil {
			log.Panic(err_commit)
			return
		}

		log.Println("Push to remote")
		_, err_push := execCmdWithOutput(config.GitCMD, "push", "origin", config.Branch)
		if err_push != nil {
			log.Panic(err_push)
			return
		}
		log.Println("Commit and push succeed")
	}

	syscall.Chdir(config.TempDir)
	mutexInRepoOp.Unlock()
}

// Change the content of the file by applying the pattern
func handleFile(config *Config, fileName string) {
	log.Println("Edit", fileName)
	new_content, err_sed := execCmdWithOutput(config.SedCMD, "-e", "s/"+config.ReplaceFrom+"/"+config.ReplaceTo+"/g", fileName)
	if err_sed != nil {
		log.Panic(err_sed)
		return
	}

	file, err := os.OpenFile(fileName, os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal("File cannot opened", fileName)
	}

	file.Truncate(0)
	file.WriteString(new_content)
	file.Close()
}

// Application script
func execute(config *Config) {
	c := oauth2.NewClient(oauth2.NoContext, &GithubTokenSource{config.PersonalToken})
	client := github.NewClient(c)

	repos := fetchRepos(config, client)
	log.Println("Found", len(repos), "repositories")

	var wg sync.WaitGroup
	for _, repo := range repos {
		wg.Add(1)
		currentRepo := repo
		go func() {
			defer wg.Done()
			isMatch, err_match := regexp.MatchString(config.RepoPattern, *currentRepo.SSHURL)
			if err_match != nil {
				log.Panic(err_match)
				return
			}

			if isMatch {
				handleRepo(config, *currentRepo.SSHURL)
			} else {
				log.Println("Ignored:", *currentRepo.SSHURL)
			}
		}()
	}
	wg.Wait()
}

func main() {
	log.Println("Repository bundle update script - START")

	flagCommit = len(os.Args) == 2 && os.Args[1] == "commit"

	config, err := getConfig()
	if err != nil {
		log.Fatalln("Configuration not found")
	}

	execute(config)
}
