package main

import (
	"encoding/base64"
	"io/ioutil"

	"github.com/pkg/errors"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

type (
	Repo struct {
		Owner string
		Name  string
	}

	Build struct {
		Tag     string
		Event   string
		Number  int
		Commit  string
		Ref     string
		Branch  string
		Author  string
		Status  string
		Link    string
		Started int64
		Created int64
	}

	Job struct {
		Started int64
	}

	Config struct {
		Ca        string
		Server    string
		Token     string
		Namespace string
		Template  string
	}

	Plugin struct {
		Repo   Repo
		Build  Build
		Config Config
		Job    Job
	}
)

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)

func (p Plugin) Exec() error {

	if p.Config.Server == "" {
		return errors.New("KUBE_SERVER is not defined")
	}
	if p.Config.Token == "" {
		return errors.New("KUBE_TOKEN is not defined")
	}
	if p.Config.Ca == "" {
		return errors.New("KUBE_CA is not defined")
	}
	if p.Config.Template == "" {
		return errors.New("KUBE_TEMPLATE, or template must be defined")
	}

	// connect to Kubernetes
	clientset, err := p.createKubeClient()
	if err != nil {
		return errors.Wrap(err, "can't create kubernetes client")
	}

	// parse the template file and do substitutions
	txt, err := openAndSub(p.Config.Template, p)
	if err != nil {
		return errors.Wrap(err, "can't read provided deployment template")
	}

	var dep appsV1.Deployment

	err = runtime.DecodeInto(codecs.UniversalDecoder(), []byte(txt), &dep)
	if err != nil {
		return errors.Wrap(err, "can't decode provided deployment template")
	}

	//override deployment namespace
	if p.Config.Namespace != "" {
		dep.Namespace = p.Config.Namespace
	}

	//set default namespace if not set
	if dep.Namespace == "" {
		dep.Namespace = coreV1.NamespaceDefault
	}

	// check and see if there is a deployment already.  If there is, update it.
	oldDep, err := findDeployment(dep.Name, dep.Namespace, clientset)
	if err != nil {
		return errors.Wrap(err, "can't read deployments")
	}
	if oldDep.Name == dep.Name {
		// update the existing deployment, ignore the deployment that it comes back with
		_, err = clientset.AppsV1().Deployments(dep.Namespace).Update(&dep)
		return errors.Wrap(err, "can't update provided deployment")
	}
	// create the new deployment since this never existed.
	_, err = clientset.AppsV1().Deployments(dep.Namespace).Create(&dep)

	return errors.Wrap(err, "can't create provided deployment")
}

func findDeployment(depName string, namespace string, c *kubernetes.Clientset) (appsV1.Deployment, error) {
	var d appsV1.Deployment
	deployments, err := c.AppsV1().Deployments(namespace).List(metaV1.ListOptions{})
	if err != nil {
		return d, err
	}

	for _, thisDep := range deployments.Items {
		if thisDep.Name == depName {
			return thisDep, nil
		}
	}
	return d, nil
}

// open up the template and then sub variables in. Handlebar stuff.
func openAndSub(templateFile string, p Plugin) (string, error) {
	t, err := ioutil.ReadFile(templateFile)
	if err != nil {
		return "", err
	}
	return RenderTrim(string(t), p)
}

// create the connection to kubernetes based on parameters passed in.
func (p Plugin) createKubeClient() (*kubernetes.Clientset, error) {

	ca, err := base64.StdEncoding.DecodeString(p.Config.Ca)
	if err != nil {
		return nil, err
	}

	config := api.NewConfig()
	config.Clusters["drone"] = &api.Cluster{
		Server: p.Config.Server,
		CertificateAuthorityData: ca,
	}
	config.AuthInfos["drone"] = &api.AuthInfo{
		Token: p.Config.Token,
	}

	config.Contexts["drone"] = &api.Context{
		Cluster:  "drone",
		AuthInfo: "drone",
	}
	config.CurrentContext = "drone"

	clientBuilder := clientcmd.NewNonInteractiveClientConfig(*config, "drone", &clientcmd.ConfigOverrides{}, nil)
	actualCfg, err := clientBuilder.ClientConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(actualCfg)
}
