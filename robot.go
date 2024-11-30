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
	CreatePRComment(org, repo, number, comment string) (success bool)
	GetPullRequestLabels(org, repo, number string) (result []string, success bool)
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

func (bot *robot) GetConfigmap() config.Configmap {
	return bot.cnf
}

func (bot *robot) RegisterEventHandler(p framework.HandlerRegister) {
	p.RegisterPullRequestHandler(bot.handlePullRequestEvent)
	p.RegisterPullRequestCommentHandler(bot.handlePullRequestCommentEvent)
}

func (bot *robot) GetLogger() *logrus.Entry {
	return bot.log
}

var (
	// a compiled regular expression for the comment that uses to check CLA sign state
	regexpCheckCLAComment = regexp.MustCompile(`(?mi)^/check-cla$`)
	// a compiled regular expression for the comment that uses to remove CLA label
	regexpCancelCLAComment = regexp.MustCompile(`(?mi)^/cla cancel$`)
	userMarkFormat         = ""

	// a placeholder string for the committer name
	placeholderCommitter = ""
	// the value from configuration.CommentNoPermissionOperateIssue
	commentCommandTrigger = ""
	// the value from configuration.CommentIssueNeedsLinkPR
	commentPRNoCommits = ""
	// the value from configuration.CommentListLinkingPullRequestsFailure
	commentAllSigned    = ""
	commentSomeNeedSign = ""
	// the value from configuration.CommentNoPermissionOperatePR
	commentUpdateLabelFailed     = ""
	placeholderCLASignGuideTitle = ""
)

func (bot *robot) handlePullRequestEvent(evt *client.GenericEvent, cnf config.Configmap, logger *logrus.Entry) {
	org, repo, number := utils.GetString(evt.Org), utils.GetString(evt.Repo), utils.GetString(evt.Number)
	repoCnf := bot.cnf.get(org, repo)
	// If the specified repository not match any repository  in the repoConfig list, it logs the warning and returns
	if repoCnf == nil {
		logger.Warningf("no config for the repo: " + org + "/" + repo)
		return
	}

	// Checks if PR is firstly created or PR source code is updated
	if !(bot.cli.CheckIfPRCreateEvent(evt) || bot.cli.CheckIfPRSourceCodeUpdateEvent(evt)) {
		return
	}

	bot.checkIfAllSignedCLA(org, repo, number, repoCnf, logger)
}

func (bot *robot) checkIfAllSignedCLA(org, repo, number string, repoCnf *repoConfig, logger *logrus.Entry) {

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

	prLabels, _ := bot.cli.GetPullRequestLabels(org, repo, number)
	allSigned, signResult := bot.checkCLASignResult(org, repo, number, commits, repoCnf)
	if allSigned {
		bot.passCLASignature(org, repo, number, signResult[0], prLabels, repoCnf)
	} else {
		bot.waitCLASignature(org, repo, number, signResult[1], prLabels, repoCnf)
	}
}

func (bot *robot) checkCLASignResult(org, repo, number string,
	commits []client.PRCommit, repoCnf *repoConfig) (allSigned bool, signResult [3][]string) {
	users, emails := bot.ListContributorNameAndEmail(commits, repoCnf)
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

	if len(unknownUsers) != 0 {
		bot.cli.CreatePRComment(org, repo, number, commentCommandTrigger)
		signResult[2] = unknownUsers
		return
	}

	if len(unsignedUsers) != 0 {
		signResult[1] = unsignedUsers
		return
	}

	signResult[0] = signedUsers
	allSigned = len(signedUsers) == len(emails)
	return
}

func (bot *robot) ListContributorNameAndEmail(commits []client.PRCommit, repoCnf *repoConfig) ([]string, []string) {
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

func (bot *robot) removeCLASignGuideComment(org, repo, number string) {
	comments, success := bot.cli.ListPullRequestComments(org, repo, number)
	if !success {
		return
	}

	for i := range comments {
		if strings.Contains(comments[i].Body, placeholderCLASignGuideTitle) {
			bot.cli.DeletePRComment(org, repo, comments[i].ID)
			break
		}
	}
}

func (bot *robot) passCLASignature(org, repo, number string, signedUsers, prLabels []string, repoCnf *repoConfig) {

	if slices.Contains(prLabels, repoCnf.CLALabelNo) {
		if !bot.cli.RemovePRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelNo)}) {
			bot.cli.CreatePRComment(org, repo, number, commentUpdateLabelFailed)
		}
	}

	if !slices.Contains(prLabels, repoCnf.CLALabelYes) {
		comment := commentUpdateLabelFailed
		if bot.cli.AddPRLabels(org, repo, number, []string{repoCnf.CLALabelYes}) {
			signedUserMark := make([]string, len(signedUsers))
			for i, user := range signedUsers {
				signedUserMark[i] = strings.ReplaceAll(userMarkFormat, placeholderCommitter, user)
			}
			comment = strings.ReplaceAll(commentAllSigned, placeholderCommitter, strings.Join(signedUserMark, ", "))
			bot.removeCLASignGuideComment(org, repo, number)
		}
		bot.cli.CreatePRComment(org, repo, number, comment)
	}
}

func (bot *robot) waitCLASignature(org, repo, number string, unsignedUsers, prLabels []string, repoCnf *repoConfig) {
	if len(unsignedUsers) == 0 {
		return
	}

	if slices.Contains(prLabels, repoCnf.CLALabelYes) {
		if !bot.cli.RemovePRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelYes)}) {
			bot.cli.CreatePRComment(org, repo, number, commentUpdateLabelFailed)
		}
	}

	if !slices.Contains(prLabels, repoCnf.CLALabelNo) {
		comment := commentUpdateLabelFailed
		if bot.cli.AddPRLabels(org, repo, number, []string{repoCnf.CLALabelNo}) {
			unsignedUserMark := make([]string, len(unsignedUsers))
			for i, user := range unsignedUsers {
				unsignedUserMark[i] = strings.ReplaceAll(userMarkFormat, placeholderCommitter, user)
			}
			comment = strings.ReplaceAll(commentSomeNeedSign, placeholderCommitter, strings.Join(unsignedUserMark, ", "))
			bot.removeCLASignGuideComment(org, repo, number)
		}
		bot.cli.CreatePRComment(org, repo, number, comment)
	}
}

func (bot *robot) handlePullRequestCommentEvent(evt *client.GenericEvent, cnf config.Configmap, logger *logrus.Entry) {
	org, repo, number := utils.GetString(evt.Org), utils.GetString(evt.Repo), utils.GetString(evt.Number)
	repoCnf := bot.cnf.get(org, repo)
	// If the specified repository not match any repository  in the repoConfig list, it logs the warning and returns
	if repoCnf == nil {
		logger.Warningf("no config for the repo: " + org + "/" + repo)
		return
	}

	comment := utils.GetString(evt.Comment)
	// Checks if the comment is only "/cla cancel" that can be handled
	if regexpCancelCLAComment.MatchString(comment) {
		prLabels, _ := bot.cli.GetPullRequestLabels(org, repo, number)
		if slices.Contains(prLabels, repoCnf.CLALabelYes) {
			bot.cli.RemovePRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelNo)})
		}
		return
	}

	// Checks if the comment is only "/check-cla" that can be handled
	if !regexpCheckCLAComment.MatchString(comment) {
		return
	}

	bot.checkIfAllSignedCLA(org, repo, number, repoCnf, logger)
}
