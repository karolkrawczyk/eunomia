package gitopsconfig

import (
	"context"
	gitopsv1alpha1 "github.com/KohlsTechnology/eunomia/pkg/apis/eunomia/v1alpha1"
	"github.com/KohlsTechnology/eunomia/pkg/util"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func updateRepo(config *gitopsv1alpha1.GitOpsConfig, client client.Client) (result bool, e error){
	ctx := context.TODO()
	githubSecret := &corev1.Secret{}
	err := client.Get(ctx, util.NN{Name: config.Spec.TemplateSource.SecretRef, Namespace: config.Namespace}, githubSecret)
	if err != nil {
		log.Error(err, "unable to retrieve secretRef from GitOpsConfig: %s", config.Name)
	}
	token := ""
	data := githubSecret.Data
	if val, ok := data["token"]; ok {
		token = string(val)
	} else {
		log.Error(err, "unable to retrieve token from secretRef")
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
		)

	tc := oauth2.NewClient(ctx, ts)
	githubClient := github.NewClient(tc)
	repoOwner, repositoryName := getOwnerAndRepo(config.Spec.TemplateSource.URI)
	lastRef, response, err := githubClient.Git.ListRefs(ctx, repoOwner, repositoryName, nil)
	if err != nil && response != nil && response.StatusCode > 299 {
		return false, err
	}
	log.Info("response", "response", response.Status)
	log.Info("lastRef", "lastCommit.SHA", lastRef)

	lastCommit, response, err := githubClient.Git.GetCommit(ctx, repoOwner, repositoryName, *lastRef[0].Object.SHA)
	if err != nil && response != nil && response.StatusCode > 299 {
		return false, err
	}

	log.Info("lastCommit", "lastCommit", lastCommit)

	*lastRef[0].Object.SHA = *lastCommit.Parents[0].SHA
	updatedRef, response, err := githubClient.Git.UpdateRef(ctx, repoOwner, repositoryName, lastRef[0], true)
	log.Info("updatedRef", "updated ref", updatedRef)
	log.Info("response after delete", "response", response)
	log.Error(err, "error")

	return true, nil
}

func getOwnerAndRepo(githubUrl string) (owner string, repo string) {
	splitUrl := strings.Split(githubUrl, "/")
	owner = splitUrl[len(splitUrl) - 2]
	repo = splitUrl[len(splitUrl) - 1]
	return owner, repo
}

