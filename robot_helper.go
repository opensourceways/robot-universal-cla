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
	"github.com/sirupsen/logrus"
	"net/url"
	"slices"
	"strings"
)

func (bot *robot) checkIfAllSignedCLA(org, repo, number string, repoCnf *repoConfig, logger *logrus.Entry) {

	commits, success := bot.cli.GetPullRequestCommits(org, repo, number)
	if !success {
		bot.cli.CreatePRComment(org, repo, number, bot.cnf.CommentCommandTrigger)
		return
	}

	if len(commits) == 0 {
		bot.cli.CreatePRComment(org, repo, number, bot.cnf.CommentPRNoCommits)
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
		bot.cli.CreatePRComment(org, repo, number, bot.cnf.CommentCommandTrigger)
		signResult[2] = unknownUsers
		return
	}

	if len(unsignedUsers) != 0 {
		signResult[1] = unsignedUsers
		return
	}

	signResult[0] = signedUsers
	allSigned = len(emails) != 0 && len(signedUsers) == len(emails)
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

func (bot *robot) passCLASignature(org, repo, number string, signedUsers, prLabels []string, repoCnf *repoConfig) {

	if slices.Contains(prLabels, repoCnf.CLALabelNo) {
		if !bot.cli.RemovePRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelNo)}) {
			bot.cli.CreatePRComment(org, repo, number, bot.cnf.CommentUpdateLabelFailed)
		}
	}

	comment := bot.cnf.CommentUpdateLabelFailed
	if bot.cli.AddPRLabels(org, repo, number, []string{repoCnf.CLALabelYes}) {
		signedUserMark := make([]string, len(signedUsers))
		for i, user := range signedUsers {
			signedUserMark[i] = strings.ReplaceAll(bot.cnf.UserMarkFormat, bot.cnf.PlaceholderCommitter, user)
		}
		comment = strings.ReplaceAll(bot.cnf.CommentAllSigned, bot.cnf.PlaceholderCommitter,
			strings.Join(signedUserMark, ", "))
		bot.removeCLASignGuideComment(org, repo, number)
	}
	bot.cli.CreatePRComment(org, repo, number, comment)

}

func (bot *robot) waitCLASignature(org, repo, number string, unsignedUsers, prLabels []string, repoCnf *repoConfig) {
	if len(unsignedUsers) == 0 {
		return
	}

	if slices.Contains(prLabels, repoCnf.CLALabelYes) {
		if !bot.cli.RemovePRLabels(org, repo, number, []string{url.QueryEscape(repoCnf.CLALabelYes)}) {
			bot.cli.CreatePRComment(org, repo, number, bot.cnf.CommentUpdateLabelFailed)
		}
	}

	comment := bot.cnf.CommentUpdateLabelFailed
	if bot.cli.AddPRLabels(org, repo, number, []string{repoCnf.CLALabelNo}) {
		unsignedUserMark := make([]string, len(unsignedUsers))
		for i, user := range unsignedUsers {
			unsignedUserMark[i] = strings.ReplaceAll(bot.cnf.UserMarkFormat, bot.cnf.PlaceholderCommitter, user)
		}
		comment = fmt.Sprintf(bot.cnf.CommentSomeNeedSign, strings.Join(unsignedUserMark, ", "),
			repoCnf.SignURL, repoCnf.FAQURL)
		bot.removeCLASignGuideComment(org, repo, number)
	}
	bot.cli.CreatePRComment(org, repo, number, comment)

}

func (bot *robot) removeCLASignGuideComment(org, repo, number string) {
	comments, success := bot.cli.ListPullRequestComments(org, repo, number)
	if !success {
		return
	}

	for i := range comments {
		if strings.Contains(comments[i].Body, bot.cnf.PlaceholderCLASignGuideTitle) ||
			strings.Contains(comments[i].Body, bot.cnf.PlaceholderCLASignPassTitle) {
			bot.cli.DeletePRComment(org, repo, comments[i].ID)
		}
	}
}
