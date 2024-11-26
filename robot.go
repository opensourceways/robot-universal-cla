// Copyright (c) Huawei Technologies Co., Ltd. 2024. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"errors"
	"github.com/opensourceways/robot-framework-lib/client"
	"github.com/opensourceways/robot-framework-lib/config"
	"github.com/opensourceways/robot-framework-lib/framework"
	"github.com/opensourceways/robot-framework-lib/utils"
	"github.com/sirupsen/logrus"
	"regexp"
	"slices"
	"strings"
)

// iClient is an interface that defines methods for client-side interactions
type iClient interface {
	// CreatePRComment creates a comment for a pull request in a specified organization and repository
	CreatePRComment(org, repo, number, comment string) (success bool)
	// CreateIssueComment creates a comment for an issue in a specified organization and repository
	CreateIssueComment(org, repo, number, comment string) (success bool)
	// CheckPermission checks the permission of a user for a specified repository
	CheckPermission(org, repo, username string) (pass, success bool)
	// UpdateIssue updates the state of an issue in a specified organization and repository
	UpdateIssue(org, repo, number, state string) (success bool)
	// UpdatePR updates the state of a pull request in a specified organization and repository
	UpdatePR(org, repo, number, state string) (success bool)
	// GetIssueLinkedPRNumber retrieves the number of a pull request linked to a specified issue
	GetIssueLinkedPRNumber(org, repo, number string) (num int, success bool)

	CreateRepoIssueLabel(org, repo, name, color string) (success bool)
	DeleteRepoIssueLabel(org, repo, name string) (success bool)
	AddIssueLabels(org, repo, number string, labels []string) (success bool)
	RemoveIssueLabels(org, repo, number string, labels []string) (success bool)
	AddPRLabels(org, repo, number string, labels []string) (success bool)
	RemovePRLabels(org, repo, number string, labels []string) (success bool)
	GetPullRequestCommits(org, repo, number string) (result []client.PRCommit, success bool)
	ListPullRequestComments(org, repo, number string) (result []client.PRComment, success bool)
	DeletePRComment(org, repo, commentID string) (success bool)
	CheckCLASignature(email string) (signState string, success bool)
}

type robot struct {
	cli iClient
	cnf *configuration
	log *logrus.Entry
}

func newRobot(c *configuration, token []byte) *robot {
	logger := framework.NewLogger().WithField("component", component)
	return &robot{cli: client.NewClient(token, logger), cnf: c, log: logger}
}

func (bot *robot) NewConfig() config.Configmap {
	return &configuration{}
}

func (bot *robot) RegisterEventHandler(p framework.HandlerRegister) {
	p.RegisterPullRequestHandler(bot.handlePullRequestEvent)
	p.RegisterPullRequestCommentHandler(bot.handlePullRequestCommentEvent)
}

func (bot *robot) GetLogger() *logrus.Entry {
	return bot.log
}

// getConfig first checks if the specified organization and repository is available in the provided repoConfig list.
// Returns an error if not found the available repoConfig.
func (bot *robot) getConfig(cnf config.Configmap, org, repo string) (*repoConfig, error) {
	c := cnf.(*configuration)
	if bc := c.get(org, repo); bc != nil {
		return bc, nil
	}

	return nil, errors.New("no config for this repo: " + org + "/" + repo)
}

var (
	// the value from configuration.EventStateOpened
	eventStateOpened = "opened"
	// the value from configuration.EventStateClosed
	eventStateClosed = "closed"
	// the value from configuration.CommentNoPermissionOperateIssue
	commentNoPermissionOperateIssue = ""
	// the value from configuration.CommentIssueNeedsLinkPR
	commentIssueNeedsLinkPR = ""
	// the value from configuration.CommentListLinkingPullRequestsFailure
	commentListLinkingPullRequestsFailure = ""
	// the value from configuration.CommentNoPermissionOperatePR
	commentNoPermissionOperatePR = ""
)

const (
	// placeholderCommenter is a placeholder string for the commenter's name
	placeholderCommenter = "__commenter__"
	// placeholderAction is a placeholder string for the action
	placeholderAction = "__action__"
)

var (
	// regexpReopenComment is a compiled regular expression for reopening comments
	regexpCheckCLAComment = regexp.MustCompile(`(?mi)^/check-cla\s*$`)
	// regexpCloseComment is a compiled regular expression for closing comments
	regexpCancelCLAComment = regexp.MustCompile(`(?mi)^/cla cancel\s*$`)
)

func (bot *robot) handlePullRequestEvent(evt *client.GenericEvent, cnf config.Configmap) {
	org, repo, number := utils.GetString(evt.Org), utils.GetString(evt.Repo), utils.GetString(evt.Number)
	repoCnf, err := bot.getConfig(cnf, org, repo)
	// If the specified repository not match any repository  in the repoConfig list, it logs the error and returns
	if err != nil {
		bot.log.WithError(err).Error()
		return
	}

	// Checks if PR is first created or PR source code is updated
	if utils.GetString(evt.State) != "opened" || !(utils.GetString(evt.Action) == "open" || utils.GetString(evt.Action) == "update") {
		return
	}

	bot.checkCLASignState(org, repo, number, repoCnf)
}

func (bot *robot) checkCLASignState(org, repo, number string, repoCnf *repoConfig) {

	//
	commits, success := bot.cli.GetPullRequestCommits(org, repo, number)
	if !success {
		return
	}

	if len(commits) == 0 {
		return
	}

	emails, users := make([]string, 0, len(commits)), make([]string, 0, len(commits))
	size := 0
	comments := ""
	allSigned := true
	for _, s := range commits {
		if repoCnf.CheckByCommitter {
			if !slices.Contains(emails[:size], s.CommitterEmail) {
				if repoCnf.LitePRCommitter.Email != s.CommitterEmail {
					allSigned = false
					break
				}
				emails[size] = s.AuthorEmail
				users[size] = s.AuthorName
				size++
			} else {
				continue
			}
		} else {
			if !slices.Contains(emails[:size], s.AuthorEmail) {
				emails[size] = s.AuthorEmail
				users[size] = s.AuthorName
				size++
			} else {
				continue
			}
		}

		signState, _ := bot.cli.CheckCLASignature(s.AuthorEmail)
		if signState != client.CLASignStateYes {
			comments += signState
			allSigned = false
		}
	}

	bot.deleteCLASignGuideComment(org, repo, number)

	if allSigned {
		bot.passCLASignature(users, org, repo, number, repoCnf)
	} else {
		bot.waitCLASignature(users, org, repo, number, repoCnf)
	}
}

func (bot *robot) deleteCLASignGuideComment(org, repo, number string) {
	comments, success := bot.cli.ListPullRequestComments(org, repo, number)
	if !success {
		return
	}

	for i := range comments {
		if strings.Contains(comments[i].Body, "| CLA Signature Guide |") {
			bot.cli.DeletePRComment(org, repo, comments[i].ID)
		}
	}
}

func (bot *robot) passCLASignature(committer []string, org, repo, number string, repoCnf *repoConfig) {
	comment := ""
	if bot.cli.RemovePRLabels(org, repo, number, []string{repoCnf.CLALabelNo}) {
		if bot.cli.AddPRLabels(org, repo, number, []string{repoCnf.CLALabelYes}) {
			comment = "ccccc"
		} else {
			comment = "aaa"
		}
	} else {
		comment = "fff"
	}
	bot.cli.CreatePRComment(org, repo, number, comment)
}

func (bot *robot) waitCLASignature(committer []string, org, repo, number string, repoCnf *repoConfig) {
	comment := ""
	if bot.cli.RemovePRLabels(org, repo, number, []string{repoCnf.CLALabelYes}) {
		if bot.cli.AddPRLabels(org, repo, number, []string{repoCnf.CLALabelNo}) {
			comment = "ccccc"
		} else {
			comment = "aaa"
		}
	} else {
		comment = "fff"
	}
	bot.cli.CreatePRComment(org, repo, number, comment)
}

func (bot *robot) handlePullRequestCommentEvent(evt *client.GenericEvent, cnf config.Configmap) {
	org, repo, number := utils.GetString(evt.Org), utils.GetString(evt.Repo), utils.GetString(evt.Number)
	repoCnf, err := bot.getConfig(cnf, org, repo)
	// If the specified repository not match any repository  in the repoConfig list, it logs the error and returns
	if err != nil {
		bot.log.WithError(err).Error()
		return
	}

	// Checks if the comment is only "/check-cla" that can be handled
	if !regexpCheckCLAComment.MatchString(utils.GetString(evt.Comment)) {
		return
	}

	bot.checkCLASignState(org, repo, number, repoCnf)
}
