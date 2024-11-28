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
	"fmt"
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
	CheckCLASignature(urlStr string) (signState string, success bool)
	CheckIfPRCreateEvent(evt *client.GenericEvent) (yes bool)
	CheckIfPRSourceCodeUpdateEvent(evt *client.GenericEvent) (yes bool)
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

const ()

var (
	// regexpReopenComment is a compiled regular expression for reopening comments
	regexpCheckCLAComment = regexp.MustCompile(`(?mi)^/check-cla$`)
	// regexpCloseComment is a compiled regular expression for closing comments
	regexpCancelCLAComment = regexp.MustCompile(`(?mi)^/cla cancel$`)
	userMarkFormat         = ""

	// placeholderCommitter is a placeholder string for the commenter's name
	placeholderCommitter = ""
	// the value from configuration.CommentNoPermissionOperateIssue
	commentCommandTrigger = ""
	// the value from configuration.CommentIssueNeedsLinkPR
	commentPRNoCommits = ""
	// the value from configuration.CommentListLinkingPullRequestsFailure
	commentAllSigned    = ""
	commentSomeNeedSign = ""
	// the value from configuration.CommentNoPermissionOperatePR
	commentUpdateLabelFailed = ""
)

func (bot *robot) handlePullRequestEvent(evt *client.GenericEvent, cnf config.Configmap, logger *logrus.Entry) {
	org, repo, number := utils.GetString(evt.Org), utils.GetString(evt.Repo), utils.GetString(evt.Number)
	repoCnf, err := bot.getConfig(cnf, org, repo)
	// If the specified repository not match any repository  in the repoConfig list, it logs the error and returns
	if err != nil {
		logger.WithError(err).Warning()
		return
	}

	// Checks if PR is first created or PR source code is updated
	if !(bot.cli.CheckIfPRCreateEvent(evt) || bot.cli.CheckIfPRSourceCodeUpdateEvent(evt)) {
		return
	}

	bot.checkIfCLASigned(org, repo, number, repoCnf, logger)
}

func (bot *robot) checkIfCLASigned(org, repo, number string, repoCnf *repoConfig, logger *logrus.Entry) {

	// List
	commits, success := bot.cli.GetPullRequestCommits(org, repo, number)
	if !success {
		bot.cli.CreatePRComment(org, repo, number, commentCommandTrigger)
		return
	}

	if len(commits) == 0 {
		bot.cli.CreatePRComment(org, repo, number, commentPRNoCommits)
		return
	}

	if allSigned, unsignedUsers, signedUsers := bot.checkCLASignedResult(org, repo, number, commits, repoCnf); allSigned {
		bot.passCLASignature(org, repo, number, signedUsers, repoCnf)
	} else {
		bot.waitCLASignature(org, repo, number, unsignedUsers, repoCnf)
	}
}

func (bot *robot) checkCLASignedResult(org, repo, number string,
	commits []client.PRCommit, repoCnf *repoConfig) (bool, string, string) {

	users, emails := bot.ListCodeContributorNameAndEmail(commits, repoCnf)
	var signedUsers, unsignedUsers, unknownUsers []string
	for i, email := range emails {
		if repoCnf.LitePRCommitter.Email == email || email == "" {
			unknownUsers = append(unknownUsers, users[i])
			continue
		}

		urlStr := fmt.Sprintf("%s?email=%s", repoCnf.CheckURL, email)
		signState, _ := bot.cli.CheckCLASignature(urlStr)
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
		bot.cli.CreatePRComment(org, repo, number, commentCommandTrigger)
		return false, "", ""
	}

	if len(unsignedUsers) != 0 {
		unsignedUserMark := make([]string, len(unsignedUsers))
		for i, user := range unsignedUsers {
			unsignedUserMark[i] = strings.ReplaceAll(userMarkFormat, placeholderCommitter, user)
		}
		return false, strings.Join(unsignedUserMark, ", "), ""
	}

	signedUserMark := make([]string, len(signedUsers))
	for i, user := range signedUsers {
		signedUserMark[i] = strings.ReplaceAll(userMarkFormat, placeholderCommitter, user)
	}
	return allSigned, "", strings.Join(signedUserMark, ", ")
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

func (bot *robot) passCLASignature(org, repo, number, signedUsers string, repoCnf *repoConfig) {
	comment := commentUpdateLabelFailed
	if bot.cli.RemovePRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelNo)}) {
		if bot.cli.AddPRLabels(org, repo, number, []string{repoCnf.CLALabelYes}) {
			comment = strings.ReplaceAll(commentAllSigned, placeholderCommitter, signedUsers)
			bot.deleteCLASignGuideComment(org, repo, number)
		}
	}
	bot.cli.CreatePRComment(org, repo, number, comment)
}

func (bot *robot) waitCLASignature(org, repo, number, unsignedUsers string, repoCnf *repoConfig) {
	if unsignedUsers == "" {
		return
	}
	comment := commentUpdateLabelFailed
	if bot.cli.RemovePRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelYes)}) {
		if bot.cli.AddPRLabels(org, repo, number, []string{repoCnf.CLALabelNo}) {
			comment = strings.ReplaceAll(commentSomeNeedSign, placeholderCommitter, unsignedUsers)
		}
	}
	bot.cli.CreatePRComment(org, repo, number, comment)
}

func (bot *robot) handlePullRequestCommentEvent(evt *client.GenericEvent, cnf config.Configmap, logger *logrus.Entry) {
	org, repo, number := utils.GetString(evt.Org), utils.GetString(evt.Repo), utils.GetString(evt.Number)
	repoCnf, err := bot.getConfig(cnf, org, repo)
	// If the specified repository not match any repository  in the repoConfig list, it logs the error and returns
	if err != nil {
		logger.WithError(err).Warning()
		return
	}

	// Checks if the comment is only "/check-cla" that can be handled
	if !regexpCheckCLAComment.MatchString(utils.GetString(evt.Comment)) {
		return
	}

	bot.checkIfCLASigned(org, repo, number, repoCnf, logger)
}
