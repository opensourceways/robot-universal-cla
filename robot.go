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
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// iClient is an interface that defines methods for client-side interactions
type iClient interface {
	// CreatePRComment creates a comment for a pull request in a specified organization and repository
	CreatePRComment(org, repo, number, comment string) (success bool)

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
	regexpCheckCLAComment = regexp.MustCompile(`(?mi)^/check-cla$`)
	// regexpCloseComment is a compiled regular expression for closing comments
	regexpCancelCLAComment = regexp.MustCompile(`(?mi)^/cla cancel$`)
	userMarkFormat         = ""
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

	// List
	commits, success := bot.cli.GetPullRequestCommits(org, repo, number)
	if !success {
		bot.cli.CreatePRComment(org, repo, number, "need manual touch off check cla")
		return
	}

	if len(commits) == 0 {
		return
	}

	if bot.checkCLASignedResult(org, repo, number, commits, repoCnf) {
		bot.passCLASignature(org, repo, number, repoCnf)
	} else {
		bot.waitCLASignature(org, repo, number, repoCnf)
	}
}

func (bot *robot) checkCLASignedResult(org, repo, number string, commits []client.PRCommit, repoCnf *repoConfig) bool {
	users, emails := bot.ListCodeContributorNameAndEmail(commits, repoCnf)
	var signedUsers, unsignedUsers, unknownUsers []string
	for i, email := range emails {
		if repoCnf.LitePRCommitter.Email == email || email == "" {
			unknownUsers = append(unknownUsers, users[i])
			continue
		}

		signState, _ := bot.cli.CheckCLASignature(email)
		switch signState {
		case client.CLASignStateYes:
			signedUsers = append(signedUsers, users[i])
		case client.CLASignStateNo:
			unsignedUsers = append(unsignedUsers, users[i])
		default:
			unknownUsers = append(unknownUsers, users[i])
		}
	}

	allSigned := len(signedUsers) == len(emails)

	if len(unknownUsers) != 0 {
		bot.cli.CreatePRComment(org, repo, number, "/check cla again")
	}

	if len(unsignedUsers) != 0 {
		bot.cli.CreatePRComment(org, repo, number, "some people need signed CLA")
	}

	return allSigned
}

func (bot *robot) ListCodeContributorNameAndEmail(commits []client.PRCommit, repoCnf *repoConfig) ([]string, []string) {
	n := len(commits)
	authors, authorEmails, authorSize := make([]string, n), make([]string, n), 0
	committers, committerEmails, committerSize := make([]string, n), make([]string, n), 0
	for i := 0; i < n; i++ {
		if !slices.Contains(authorEmails[:authorSize], commits[i].AuthorEmail) {
			authorEmails[authorSize] = commits[i].AuthorEmail
			authors[authorSize] = commits[i].AuthorName
			authorSize++
		}
		if !slices.Contains(committerEmails[:committerSize], commits[i].CommitterEmail) {
			committerEmails[committerSize] = commits[i].CommitterEmail
			committers[committerSize] = commits[i].CommitterName
			committerSize++
		}
	}
	if repoCnf.CheckByCommitter {
		return committers[:committerSize], committerEmails[:committerSize]
	}

	return authors[:authorSize], authorEmails[:authorSize]
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

func (bot *robot) passCLASignature(org, repo, number string, repoCnf *repoConfig) {

	comment := "no CLA label add fail, once again "
	if bot.cli.RemovePRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelNo)}) {
		if bot.cli.AddPRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelYes)}) {
			comment = "all signed "
			bot.deleteCLASignGuideComment(org, repo, number)
		} else {
			comment = "yes CLA label add fail, once again "
		}
	}
	bot.cli.CreatePRComment(org, repo, number, comment)
}

func (bot *robot) waitCLASignature(org, repo, number string, repoCnf *repoConfig) {
	comment := "yes CLA label remove fail, once again "
	if bot.cli.RemovePRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelYes)}) {
		if bot.cli.AddPRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelNo)}) {
			comment = "wait people sign CLA"
		} else {
			comment = "no CLA label add fail, once again "
		}
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
