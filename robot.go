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
	regexpReopenComment = regexp.MustCompile(`(?mi)^/reopen\s*$`)
	// regexpCloseComment is a compiled regular expression for closing comments
	regexpCloseComment = regexp.MustCompile(`(?mi)^/close\s*$`)
)

func (bot *robot) handlePullRequestEvent(evt *client.GenericEvent, cnf config.Configmap) {
	org, repo, number := utils.GetString(evt.Org), utils.GetString(evt.Repo), utils.GetString(evt.Number)
	configmap, err := bot.getConfig(cnf, org, repo)
	// If the specified repository not match any repository  in the repoConfig list, it logs the error and returns
	if err != nil {
		bot.log.WithError(err).Error()
		return
	}

	// C

	// Checks if PR source code update

	if bot.checkCLASignState(org, repo, number) {
		return
	}

	if intersection := checkIntersection(toAdd, toRemove); len(intersection) != 0 {

		return
	}

	bot.addIssueLabels(org, repo, number, toAdd)
	bot.removeIssueLabels(org, repo, number, toAdd)

}

func (bot *robot) checkCLASignState(org, repo, number string) (interrupt bool) {

	//
	commits, success := bot.cli.GetPullRequestCommits(org, repo, number)
	if !success {
		return true
	}

	if len(commits) == 0 {
		return true
	}

	emails, users := make([]string, 0, len(commits)), make([]string, 0, len(commits))
	size := 0
	comments := ""
	signFlag := true
	for _, s := range commits {
		if !slices.Contains(emails[:size], s.AuthorEmail) {
			emails[size] = s.AuthorEmail
			users[size] = s.AuthorName
			size++
		} else {
			continue
		}
		signState, _ := bot.cli.CheckCLASignature(s.AuthorEmail)
		if signState != client.CLASignStateYes {
			comments += signState
			signFlag = false
		}
	}

	if signFlag {

	}

	deleteSignGuide
}

func checkCLASignature(add, remove []string) (intersection []string) {
	for _, r := range remove {
		if slices.Contains(add, r) {
			intersection = append(intersection, r)
		}
	}
	return
}

func (bot *robot) addIssueLabels(org, repo, number string, labels []string) {
	if len(labels) != 0 {

	}
}

func (bot *robot) removeIssueLabels(org, repo, number string, labels []string) {
	if len(labels) != 0 {

	}
}

func (bot *robot) clearPRLabelsCaseBySourceCodeUpdate(evt *client.GenericEvent, org, repo, number string) {
	bot.cli.RemovePRLabels(org, repo, number, evt.Labels)
	comment := fmt.Sprintf(
		"This pull request source branch has changed, so removes the following label(s): %s.",
		strings.Join(evt.Labels, ", "),
	)
	bot.cli.CreatePRComment(org, repo, number, comment)
}

func (bot *robot) handlePullRequestCommentEvent(evt *client.GenericEvent, cnf config.Configmap) {
	org, repo, number := utils.GetString(evt.Org), utils.GetString(evt.Repo), utils.GetString(evt.Number)
	configmap, err := bot.getConfig(cnf, org, repo)
	// If the specified repository not match any repository  in the repoConfig list, it logs the error and returns
	if err != nil {
		bot.log.WithError(err).Error()
		return
	}

	// Checks if the event can be handled as a reopen event
	if bot.handleReopenEvent(evt, org, repo, number) {
		return
	}

	// Handles the close event
	bot.handleCloseEvent(evt, configmap, org, repo, number)
}

// handleReopenEvent only handles the reopening of an issue event.
// Handle completed, set the interrupt flag to interrupt the subsequent operations.
func (bot *robot) handleReopenEvent(evt *client.GenericEvent, org, repo, number string) (interrupt bool) {
	comment, state := utils.GetString(evt.Comment), utils.GetString(evt.State)
	commenter, author := utils.GetString(evt.Commenter), utils.GetString(evt.Author)
	// If the comment is on an issue and the comment matches the reopen comment and the state is closed
	if utils.GetString(evt.CommentKind) == client.CommentOnIssue && regexpReopenComment.MatchString(comment) && state == eventStateClosed {
		interrupt = true
		// Check if the commenter has the permission to operate
		if !bot.checkCommenterPermission(org, repo, author, commenter, func() {
			bot.cli.CreateIssueComment(org, repo, number,
				strings.ReplaceAll(strings.ReplaceAll(commentNoPermissionOperateIssue, placeholderCommenter, commenter), placeholderAction, "reopen"))
		}) {
			return
		}

		bot.cli.UpdateIssue(org, repo, number, eventStateOpened)
	}
	return
}

// handleCloseEvent  handles the closing of an issue or pull request event
func (bot *robot) handleCloseEvent(evt *client.GenericEvent, configmap *repoConfig, org, repo, number string) {
	comment, state := utils.GetString(evt.Comment), utils.GetString(evt.State)
	commenter, author := utils.GetString(evt.Commenter), utils.GetString(evt.Author)
	// If the comment matches the close comment and the state is opened
	if regexpCloseComment.MatchString(comment) && state == eventStateOpened {
		// Check if the commenter has the permission to operate
		if !bot.checkCommenterPermission(org, repo, author, commenter, func() {
			if utils.GetString(evt.CommentKind) == client.CommentOnIssue {
				bot.cli.CreateIssueComment(org, repo, number,
					strings.ReplaceAll(strings.ReplaceAll(commentNoPermissionOperateIssue, placeholderCommenter, commenter), placeholderAction, "close"))
			} else {
				bot.cli.CreatePRComment(org, repo, number,
					strings.ReplaceAll(strings.ReplaceAll(commentNoPermissionOperatePR, placeholderCommenter, commenter), placeholderAction, "close"))
			}
		}) {
			return
		}

		// If the comment kind is an pull request, update the pull request state to closed and return
		if utils.GetString(evt.CommentKind) != client.CommentOnIssue {
			bot.cli.UpdatePR(org, repo, number, eventStateClosed)
			return
		}

		// Check if the issue needs linking to a pull request, and update the issue state to closed
		bot.checkIssueNeedLinkingPR(configmap, org, repo, number, commenter)
	}
}

// handleCloseEvent  handles the closing of an issue
func (bot *robot) checkIssueNeedLinkingPR(configmap *repoConfig, org, repo, number, commenter string) {
	if configmap.NeedIssueHasLinkPullRequests {
		// issue can be closed only when its linking PR exists
		num, success := bot.cli.GetIssueLinkedPRNumber(org, repo, number)
		bot.log.Infof("list the issue[%s/%s,%s] linking PR number is successful: %v, number: %d", org, repo, number, success, num)
		// If the request is failed that means not be sure to close issue, create a comment indicating do closing again and return
		if !success {
			bot.cli.CreateIssueComment(org, repo, number, strings.ReplaceAll(commentListLinkingPullRequestsFailure, placeholderCommenter, commenter))
			return
		}

		// If the linked pull request number is zero, create a comment indicating that the issue needs a linked pull request and return
		if num == 0 {
			bot.cli.CreateIssueComment(org, repo, number, strings.ReplaceAll(commentIssueNeedsLinkPR, placeholderCommenter, commenter))
			return
		}
	}

	bot.cli.UpdateIssue(org, repo, number, eventStateClosed)
}

func (bot *robot) checkCommenterPermission(org, repo, author, commenter string, fn func()) (pass bool) {
	if author == commenter {
		return true
	}
	pass, success := bot.cli.CheckPermission(org, repo, commenter)
	bot.log.Infof("request success: %t, the %s has permission to the repo[%s/%s]: %t", success, commenter, org, repo, pass)

	if success && !pass {
		fn()
	}
	return pass && success
}
