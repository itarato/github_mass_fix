Ugly GitHub Mass Updater
========================

This mass updater is intended to apply code changes (replace strings using patterns) to a set of GitHub repositories. It is using token authentication along with the GitHub API.

The script is very much finetuned to a particular usecase (explained below) - however with little tweaks it can be a handy tool for other needs where massive number or repos needs to be updated.

Workflow
--------

The script requires your GitHub token. (You can generate one on your account page.) This way it only has access to repos that you are allowed to see/edit. Then it fetches the teams for a given organisation - in order to find the one team whoes repos will be changed. Then goes through the teams repos, look for files that contain the pattern (to change) and apply the change.

You have to add ```commit``` command line argument in order to make the commit automatically.

Usage
-----

Dry run, only logs and shows diffs:

```bash
$ go run main.go
```


Run and commit:
```
$ go run main.go commit
```

Setup
-------------

Get dependencies:

```bash
$ go get github.com/google/go-github/github
$ go get golang.org/x/oauth2
```

Create your ```config.json``` (using the template from ```sample.config.json```):

```json
{
  "personalToken": "YOUR_GITHUB_TOKEN",
  "orgName": "GITHUB_ORGANIZATION_NAME",
  "teamName": "TEAM_NAME_FROM_ORGANIZATION",
  "tempDir": "TEMP_DIR_TO_PUT_CLONES",
  "gitCmd": "GIT_EXECUTABLE",
  "grepCmd": "GREP_EXECUTABLE",
  "sedCMD": "SED_EXECUTABLE",
  "repoPattern": "REPOSITORY_PATTERN_IN_CASE_YOU_NEED_TO_FILTER",
  "replaceFrom": "REPLACE_WHAT",
  "replaceTo": "REPLACE_TO",
  "branch": "BRANCH_NAME_OF_CLONED_REPOS",
  "commitMessage": "COMMIT_MESSAGE"
}
```

An example:

```json
{
  "personalToken": "a1cb1ac387a6da78d6a87d6a786f767b86c87c87",
  "orgName": "myorg",
  "teamName": "myteam",
  "tempDir": "/tmp/",
  "gitCmd": "/usr/bin/git",
  "grepCmd": "/usr/bin/grep",
  "sedCMD": "/usr/bin/sed",
  "repoPattern": "github.com:myaccountname",
  "replaceFrom": "function\\ wrong_name",
  "replaceTo": "function\\ good_name",
  "branch": "master",
  "commitMessage": "Fixes issue #foobar - xyz bug"
}
```
