package opa

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/open-policy-agent/opa/rego"
	"github.com/porter-dev/porter/api/types"
	"github.com/porter-dev/porter/internal/helm"
	"github.com/porter-dev/porter/internal/kubernetes"
	"github.com/porter-dev/porter/pkg/logger"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type KubernetesPolicies struct {
	Policies map[string]KubernetesOPAQueryCollection
}

type KubernetesOPARunner struct {
	*KubernetesPolicies

	k8sAgent      *kubernetes.Agent
	dynamicClient dynamic.Interface
}

type KubernetesBuiltInKind string

const (
	HelmRelease KubernetesBuiltInKind = "helm_release"
	Pod         KubernetesBuiltInKind = "pod"
	CRDList     KubernetesBuiltInKind = "crd_list"
)

type KubernetesOPAQueryCollection struct {
	Kind    KubernetesBuiltInKind
	Match   MatchParameters
	Queries []rego.PreparedEvalQuery
}

type MatchParameters struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`

	ChartName string `json:"chart_name"`

	Labels map[string]string `json:"labels"`

	// parameters for CRDs
	Group    string `json:"group"`
	Version  string `json:"version"`
	Resource string `json:"resource"`
}

type OPARecommenderQueryResult struct {
	Allow bool

	CategoryName string
	ObjectID     string

	PolicyVersion  string
	PolicySeverity string
	PolicyTitle    string
	PolicyMessage  string
}

type rawQueryResult struct {
	Allow          bool   `mapstructure:"ALLOW"`
	PolicyID       string `mapstructure:"POLICY_ID"`
	PolicyVersion  string `mapstructure:"POLICY_VERSION"`
	PolicySeverity string `mapstructure:"POLICY_SEVERITY"`
	PolicyTitle    string `mapstructure:"POLICY_TITLE"`
	SuccessMessage string `mapstructure:"POLICY_SUCCESS_MESSAGE"`

	FailureMessage []string `mapstructure:"FAILURE_MESSAGE"`
}

func NewRunner(policies *KubernetesPolicies, k8sAgent *kubernetes.Agent, dynamicClient dynamic.Interface) *KubernetesOPARunner {
	return &KubernetesOPARunner{policies, k8sAgent, dynamicClient}
}

func (runner *KubernetesOPARunner) GetRecommendationsByName(name string) ([]*OPARecommenderQueryResult, error) {
	// look up to determine if the name is registered
	queryCollection, exists := runner.Policies[name]

	if !exists {
		return nil, fmt.Errorf("No policies for %s found", name)
	}

	switch queryCollection.Kind {
	case HelmRelease:
		return runner.runHelmReleaseQueries(name, queryCollection)
	case Pod:
		return runner.runPodQueries(name, queryCollection)
	case CRDList:
		return runner.runCRDListQueries(name, queryCollection)
	default:
		return nil, fmt.Errorf("Not a supported query kind")
	}
}

func (runner *KubernetesOPARunner) SetK8sAgent(k8sAgent *kubernetes.Agent) {
	runner.k8sAgent = k8sAgent
}

func (runner *KubernetesOPARunner) runHelmReleaseQueries(name string, collection KubernetesOPAQueryCollection) ([]*OPARecommenderQueryResult, error) {
	res := make([]*OPARecommenderQueryResult, 0)

	helmAgent, err := helm.GetAgentFromK8sAgent("secret", collection.Match.Namespace, logger.New(false, os.Stdout), runner.k8sAgent)

	if err != nil {
		return nil, err
	}

	// get the matching helm release(s) based on the match
	var helmReleases []*release.Release

	if collection.Match.Name != "" {
		helmRelease, err := helmAgent.GetRelease(collection.Match.Name, 0, false)

		if err != nil {
			return nil, err
		}

		helmReleases = append(helmReleases, helmRelease)
	} else if collection.Match.ChartName != "" {
		prefilterReleases, err := helmAgent.ListReleases(collection.Match.Namespace, &types.ReleaseListFilter{
			ByDate: true,
			StatusFilter: []string{
				"deployed",
				"pending",
				"pending-install",
				"pending-upgrade",
				"pending-rollback",
				"failed",
			},
		})

		if err != nil {
			return nil, err
		}

		for _, prefilterRelease := range prefilterReleases {
			if prefilterRelease.Chart.Name() == collection.Match.ChartName {
				helmReleases = append(helmReleases, prefilterRelease)
			}
		}
	} else {
		return nil, fmt.Errorf("invalid match parameters")
	}

	for _, helmRelease := range helmReleases {
		for _, query := range collection.Queries {
			results, err := query.Eval(
				context.Background(),
				rego.EvalInput(map[string]interface{}{
					"version": helmRelease.Chart.Metadata.Version,
					"values":  helmRelease.Config,
				}),
			)

			if err != nil {
				return nil, err
			}

			if len(results) == 1 {
				rawQueryRes := &rawQueryResult{}

				err = mapstructure.Decode(results[0].Expressions[0].Value, rawQueryRes)

				if err != nil {
					return nil, err
				}

				res = append(res, rawQueryResToRecommenderQueryResult(
					rawQueryRes,
					fmt.Sprintf("helm_release/%s/%s/%s", helmRelease.Namespace, helmRelease.Name, rawQueryRes.PolicyID),
					name,
				))
			}
		}
	}

	return res, nil
}

func (runner *KubernetesOPARunner) runPodQueries(name string, collection KubernetesOPAQueryCollection) ([]*OPARecommenderQueryResult, error) {
	res := make([]*OPARecommenderQueryResult, 0)

	lselArr := make([]string, 0)

	for k, v := range collection.Match.Labels {
		lselArr = append(lselArr, fmt.Sprintf("%s=%s", k, v))
	}

	lsel := strings.Join(lselArr, ",")

	pods, err := runner.k8sAgent.GetPodsByLabel(lsel, collection.Match.Namespace)

	if err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		unstructuredPod, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pod)

		if err != nil {
			return nil, err
		}

		for _, query := range collection.Queries {
			results, err := query.Eval(
				context.Background(),
				rego.EvalInput(unstructuredPod),
			)

			if err != nil {
				return nil, err
			}

			if len(results) == 1 {
				rawQueryRes := &rawQueryResult{}

				err = mapstructure.Decode(results[0].Expressions[0].Value, rawQueryRes)

				if err != nil {
					return nil, err
				}

				res = append(res, rawQueryResToRecommenderQueryResult(
					rawQueryRes,
					fmt.Sprintf("pod/%s/%s", pod.Namespace, pod.Name),
					name,
				))
			}
		}
	}

	return res, nil
}

func (runner *KubernetesOPARunner) runCRDListQueries(name string, collection KubernetesOPAQueryCollection) ([]*OPARecommenderQueryResult, error) {
	res := make([]*OPARecommenderQueryResult, 0)

	objRes := schema.GroupVersionResource{
		Group:    collection.Match.Group,
		Version:  collection.Match.Version,
		Resource: collection.Match.Resource,
	}

	crdList, err := runner.dynamicClient.Resource(objRes).Namespace(collection.Match.Namespace).List(context.Background(), v1.ListOptions{})

	if err != nil {
		return nil, err
	}

	for _, crd := range crdList.Items {
		for _, query := range collection.Queries {
			results, err := query.Eval(
				context.Background(),
				rego.EvalInput(crd.Object),
			)

			if err != nil {
				return nil, err
			}

			if len(results) == 1 {
				rawQueryRes := &rawQueryResult{}

				err = mapstructure.Decode(results[0].Expressions[0].Value, rawQueryRes)

				if err != nil {
					return nil, err
				}

				res = append(res, rawQueryResToRecommenderQueryResult(
					rawQueryRes,
					fmt.Sprintf("%s/%s/%s/%s", collection.Match.Group, collection.Match.Version, collection.Match.Resource, rawQueryRes.PolicyID),
					name,
				))
			}
		}
	}

	return res, nil
}

func rawQueryResToRecommenderQueryResult(rawQueryRes *rawQueryResult, objectID, categoryName string) *OPARecommenderQueryResult {
	queryRes := &OPARecommenderQueryResult{
		ObjectID:     objectID,
		CategoryName: categoryName,
	}

	message := rawQueryRes.SuccessMessage

	// if failure, compose failure messages into single string
	if !rawQueryRes.Allow {
		message = strings.Join(rawQueryRes.FailureMessage, ". ")
	}

	queryRes.PolicyMessage = message
	queryRes.Allow = rawQueryRes.Allow
	queryRes.PolicySeverity = rawQueryRes.PolicySeverity
	queryRes.PolicyTitle = rawQueryRes.PolicyTitle
	queryRes.PolicyVersion = rawQueryRes.PolicyVersion

	return queryRes
}
