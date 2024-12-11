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
	CheckPermission(org, repo, username string) (pass, success bool)
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
	regexpCheckCLAComment = regexp.MustCompile(`^/check-cla$`)
	// a compiled regular expression for the comment that uses to remove CLA label
	regexpCancelCLAComment = regexp.MustCompile(`^/cla[\t ]+cancel$`)
)

func (bot *robot) handlePullRequestEvent(evt *client.GenericEvent, cnf config.Configmap, logger *logrus.Entry) {
	org, repo, number := utils.GetString(evt.Org), utils.GetString(evt.Repo), utils.GetString(evt.Number)
	repoCnf := bot.cnf.getRepoConfig(org, repo)
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

func (bot *robot) handlePullRequestCommentEvent(evt *client.GenericEvent, cnf config.Configmap, logger *logrus.Entry) {
	org, repo, number := utils.GetString(evt.Org), utils.GetString(evt.Repo), utils.GetString(evt.Number)
	repoCnf := bot.cnf.getRepoConfig(org, repo)
	// If the specified repository not match any repository  in the repoConfig list, it logs the warning and returns
	if repoCnf == nil {
		logger.Warningf("no config for the repo: " + org + "/" + repo)
		return
	}

	comment := strings.TrimSpace(utils.GetString(evt.Comment))
	// Checks if the comment is only "/cla cancel" that can be handled
	if regexpCancelCLAComment.MatchString(comment) {
		permissionPass, _ := bot.cli.CheckPermission(org, repo, utils.GetString(evt.Commenter))
		if permissionPass {
			prLabels, _ := bot.cli.GetPullRequestLabels(org, repo, number)
			if slices.Contains(prLabels, repoCnf.CLALabelYes) {
				bot.cli.RemovePRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelYes)})
			}
		}
		return
	}

	// Checks if the comment is only "/check-cla" that can be handled
	if !regexpCheckCLAComment.MatchString(comment) {
		return
	}

	bot.checkIfAllSignedCLA(org, repo, number, repoCnf, logger)
}
