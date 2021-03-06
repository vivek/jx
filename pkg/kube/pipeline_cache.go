package kube

import (
	"github.com/jenkins-x/jx/pkg/apis/jenkins.io/v1"
	"github.com/jenkins-x/jx/pkg/client/clientset/versioned"
	"github.com/jenkins-x/jx/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"sync"
	"time"
)

// PipelineNamespaceCache caches the pipelines for a single namespace
type PipelineNamespaceCache struct {
	pipelines sync.Map
	stop chan struct{}
}

// NewPipelineCache creates a cache of pipelines for a namespace
func NewPipelineCache(jxClient versioned.Interface, ns string) *PipelineNamespaceCache {
	pipeline := &v1.PipelineActivity{}
	pipelineListWatch := cache.NewListWatchFromClient(jxClient.JenkinsV1().RESTClient(), "pipelineactivities", ns, fields.Everything())

	namespaceCache := &PipelineNamespaceCache{
		stop: make(chan struct{}),
	}

	// lets pre-populate the cache on startup as there's not yet a way to know when the informer has completed its first list operation
	pipelines, _ := jxClient.JenkinsV1().PipelineActivities(ns).List(metav1.ListOptions{})
	if pipelines != nil {
		for _, pipeline := range pipelines.Items {
			copy := pipeline
			namespaceCache.pipelines.Store(pipeline.Name, &copy)
		}
	}
	_, pipelineController := cache.NewInformer(
		pipelineListWatch,
		pipeline,
		time.Minute*10,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				namespaceCache.onPipelineObj(obj, jxClient, ns)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				namespaceCache.onPipelineObj(newObj, jxClient, ns)
			},
			DeleteFunc: func(obj interface{}) {
				namespaceCache.onPipelineDelete(obj, jxClient, ns)
			},
		},
	)

	go pipelineController.Run(namespaceCache.stop)

	return namespaceCache
}

// Stop closes the underlying chanel processing events which stops consuming watch events
func (c *PipelineNamespaceCache) Stop() {
	close(c.stop)
}
// Pipelines returns the pipelines in this namespace sorted in name order
func (c *PipelineNamespaceCache) Pipelines() []*v1.PipelineActivity {
	answer := []*v1.PipelineActivity{}
	onEntry := func(key interface{}, value interface{}) bool {
		pipeline, ok := value.(*v1.PipelineActivity)
		if ok && pipeline != nil {
			answer = append(answer, pipeline)
		}
		return true
	}
	c.pipelines.Range(onEntry)
	return answer
}

func (c *PipelineNamespaceCache) onPipelineObj(obj interface{}, jxClient versioned.Interface, ns string) {
	pipeline, ok := obj.(*v1.PipelineActivity)
	if !ok {
		log.Warnf("Object is not a PipelineActivity %#v\n", obj)
		return
	}
	if pipeline != nil {
		c.pipelines.Store(pipeline.Name, pipeline)
	}
}

func (c *PipelineNamespaceCache) onPipelineDelete(obj interface{}, jxClient versioned.Interface, ns string) {
	pipeline, ok := obj.(*v1.PipelineActivity)
	if !ok {
		log.Warnf("Object is not a PipelineActivity %#v\n", obj)
		return
	}
	if pipeline != nil {
		c.pipelines.Delete(pipeline.Name)
	}
}
