// Package parity enforces that the security-relevant posture of the raw
// kustomize manifests under config/csi-provider (daemonset.yaml, rbac.yaml)
// stays identical to the Helm chart under config/csi-provider/chart that is
// generated from the same source of intent. There is no single source of
// truth the two are rendered from, so this test renders both with the same
// tools an operator would use (kubectl kustomize / helm template) and
// compares the fields that matter for the security posture of a component
// with access to Secrets Manager access tokens: pod/container
// securityContext, resources, tolerations, priorityClassName, volumes and
// volumeMounts, and the set of RBAC-relevant object kinds each renders.
//
// Run via `go test ./...` (also wired into CI, see .github/workflows/build.yml
// and `make verify-manifest-parity`). If kubectl or helm are not available on
// PATH, the test is skipped rather than failed, since those are external
// tools rather than Go dependencies of this module.
package parity

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

// thisDir returns the absolute directory containing this test file, so the
// test does not depend on the process's working directory.
func thisDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file location")
	}
	return filepath.Dir(file)
}

// requireTool skips the test if the named external tool is not on PATH.
func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not found on PATH, skipping manifest parity check: %v", name, err)
	}
}

// typeMeta is used to sniff the Kind of each YAML document before deciding
// how (or whether) to decode it further.
type typeMeta struct {
	Kind string `json:"kind"`
}

// splitDocs splits a multi-document YAML stream (as produced by both
// `kubectl kustomize` and `helm template`) into individual documents,
// dropping empty/comment-only documents.
func splitDocs(rendered []byte) []string {
	var docs []string
	for _, doc := range strings.Split(string(rendered), "\n---\n") {
		trimmed := strings.TrimSpace(doc)
		if trimmed == "" {
			docs = append(docs, doc)
			continue
		}
		hasContent := false
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				hasContent = true
				break
			}
		}
		if hasContent {
			docs = append(docs, doc)
		}
	}
	return docs
}

// kindCounts returns how many documents of each Kind are present.
func kindCounts(t *testing.T, docs []string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, doc := range docs {
		var tm typeMeta
		if err := yaml.Unmarshal([]byte(doc), &tm); err != nil {
			t.Fatalf("failed to parse rendered document for its kind: %v\n---\n%s", err, doc)
		}
		if tm.Kind == "" {
			continue
		}
		counts[tm.Kind]++
	}
	return counts
}

func findDoc(t *testing.T, docs []string, kind string) string {
	t.Helper()
	var found string
	matches := 0
	for _, doc := range docs {
		var tm typeMeta
		if err := yaml.Unmarshal([]byte(doc), &tm); err != nil {
			continue
		}
		if tm.Kind == kind {
			found = doc
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("expected exactly one %s document, found %d", kind, matches)
	}
	return found
}

// containerSnapshot captures the security-relevant fields of a container
// spec that must match between the two render paths.
type containerSnapshot struct {
	Name            string
	Args            []string
	SecurityContext *corev1.SecurityContext
	Resources       corev1.ResourceRequirements
	VolumeMounts    []corev1.VolumeMount
}

// daemonSetSnapshot captures the security-relevant fields of the DaemonSet
// pod spec that must match between the two render paths. Fields that are
// expected to legitimately differ (names, namespaces, labels, annotations,
// selectors, image references, ServiceAccount name under a Helm release
// prefix) are intentionally excluded.
type daemonSetSnapshot struct {
	UpdateStrategy               appsv1.DaemonSetUpdateStrategy
	PriorityClassName            string
	AutomountServiceAccountToken *bool
	Tolerations                  []corev1.Toleration
	SecurityContext              *corev1.PodSecurityContext
	Volumes                      []corev1.Volume
	Containers                   []containerSnapshot
}

func snapshotDaemonSet(t *testing.T, doc string) daemonSetSnapshot {
	t.Helper()
	var ds appsv1.DaemonSet
	if err := yaml.Unmarshal([]byte(doc), &ds); err != nil {
		t.Fatalf("failed to decode DaemonSet: %v", err)
	}

	podSpec := ds.Spec.Template.Spec

	volumes := append([]corev1.Volume{}, podSpec.Volumes...)
	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Name < volumes[j].Name })

	containers := make([]containerSnapshot, 0, len(podSpec.Containers))
	for _, c := range podSpec.Containers {
		mounts := append([]corev1.VolumeMount{}, c.VolumeMounts...)
		sort.Slice(mounts, func(i, j int) bool { return mounts[i].Name < mounts[j].Name })
		containers = append(containers, containerSnapshot{
			Name:            c.Name,
			Args:            c.Args,
			SecurityContext: c.SecurityContext,
			Resources:       c.Resources,
			VolumeMounts:    mounts,
		})
	}
	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })

	tolerations := append([]corev1.Toleration{}, podSpec.Tolerations...)
	sort.Slice(tolerations, func(i, j int) bool { return tolerations[i].Key < tolerations[j].Key })

	return daemonSetSnapshot{
		UpdateStrategy:               ds.Spec.UpdateStrategy,
		PriorityClassName:            podSpec.PriorityClassName,
		AutomountServiceAccountToken: podSpec.AutomountServiceAccountToken,
		Tolerations:                  tolerations,
		SecurityContext:              podSpec.SecurityContext,
		Volumes:                      volumes,
		Containers:                   containers,
	}
}

func snapshotServiceAccountToken(t *testing.T, doc string) *bool {
	t.Helper()
	var sa corev1.ServiceAccount
	if err := yaml.Unmarshal([]byte(doc), &sa); err != nil {
		t.Fatalf("failed to decode ServiceAccount: %v", err)
	}
	return sa.AutomountServiceAccountToken
}

func sortedRules(rules []rbacv1.PolicyRule) []rbacv1.PolicyRule {
	out := append([]rbacv1.PolicyRule{}, rules...)
	sort.Slice(out, func(i, j int) bool {
		return strings.Join(out[i].Resources, ",") < strings.Join(out[j].Resources, ",")
	})
	return out
}

func dump(t *testing.T, label string, v interface{}) string {
	t.Helper()
	b, err := yaml.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal %s for diagnostics: %v", label, err)
	}
	return label + ":\n" + string(b)
}

func TestDaemonSetSecurityPostureMatchesAcrossKustomizeAndHelm(t *testing.T) {
	requireTool(t, "kubectl")
	requireTool(t, "helm")

	dir := thisDir(t)
	kustomizeDir := filepath.Join(dir, "..")
	chartDir := filepath.Join(dir, "..", "chart")

	kustomizeOut, err := exec.Command("kubectl", "kustomize", kustomizeDir).Output()
	if err != nil {
		t.Fatalf("kubectl kustomize %s failed: %v", kustomizeDir, err)
	}

	helmCmd := exec.Command("helm", "template", "sync", chartDir,
		"--namespace", "kube-system",
		"--set", "fullnameOverride=bitwarden-csi-provider")
	var helmErr bytes.Buffer
	helmCmd.Stderr = &helmErr
	helmOut, err := helmCmd.Output()
	if err != nil {
		t.Fatalf("helm template %s failed: %v\n%s", chartDir, err, helmErr.String())
	}

	kustomizeDocs := splitDocs(kustomizeOut)
	helmDocs := splitDocs(helmOut)

	kustomizeKinds := kindCounts(t, kustomizeDocs)
	helmKinds := kindCounts(t, helmDocs)
	if !reflect.DeepEqual(kustomizeKinds, helmKinds) {
		t.Fatalf("rendered object kinds differ between kustomize and Helm -- a "+
			"resource (e.g. a Role/RoleBinding) was added to one but not the "+
			"other:\nkustomize: %v\nhelm:      %v", kustomizeKinds, helmKinds)
	}

	kustomizeDS := snapshotDaemonSet(t, findDoc(t, kustomizeDocs, "DaemonSet"))
	helmDS := snapshotDaemonSet(t, findDoc(t, helmDocs, "DaemonSet"))
	if !reflect.DeepEqual(kustomizeDS, helmDS) {
		t.Fatalf("DaemonSet security posture differs between config/csi-provider/daemonset.yaml "+
			"and config/csi-provider/chart/templates/daemonset.yaml; keep the two in sync:\n%s\n%s",
			dump(t, "kustomize", kustomizeDS), dump(t, "helm", helmDS))
	}

	kustomizeSAToken := snapshotServiceAccountToken(t, findDoc(t, kustomizeDocs, "ServiceAccount"))
	helmSAToken := snapshotServiceAccountToken(t, findDoc(t, helmDocs, "ServiceAccount"))
	if !reflect.DeepEqual(kustomizeSAToken, helmSAToken) {
		t.Fatalf("ServiceAccount automountServiceAccountToken differs between "+
			"config/csi-provider/rbac.yaml and config/csi-provider/chart/templates/serviceaccount.yaml:\n"+
			"kustomize: %v\nhelm: %v", kustomizeSAToken, helmSAToken)
	}

	// Future-proofing: if a Role/ClusterRole is ever added to either the raw
	// manifests or the chart, make sure its rules match too.
	for _, kind := range []string{"Role", "ClusterRole"} {
		if kustomizeKinds[kind] == 0 {
			continue
		}
		var kustomizeRole, helmRole rbacv1.Role
		if err := yaml.Unmarshal([]byte(findDoc(t, kustomizeDocs, kind)), &kustomizeRole); err != nil {
			t.Fatalf("failed to decode kustomize %s: %v", kind, err)
		}
		if err := yaml.Unmarshal([]byte(findDoc(t, helmDocs, kind)), &helmRole); err != nil {
			t.Fatalf("failed to decode helm %s: %v", kind, err)
		}
		if !reflect.DeepEqual(sortedRules(kustomizeRole.Rules), sortedRules(helmRole.Rules)) {
			t.Fatalf("%s rules differ between kustomize and Helm render", kind)
		}
	}
}
