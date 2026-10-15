package analyze

// This file tests usage paths and value flows created by Helm metadata.

import "testing"

func TestMarkPath(t *testing.T) {
	usage := NewUsage()
	usage.MarkPath("example/Chart.yaml", "child", "enabled")

	child := usage.Properties["child"]
	if child == nil || child.Properties["enabled"] == nil || !child.Properties["enabled"].Read {
		t.Fatalf("marked usage = %#v", usage)
	}
	if sources := child.Properties["enabled"].Sources; len(sources) != 1 || sources[0] != "example/Chart.yaml" {
		t.Fatalf("metadata source = %#v", sources)
	}
}

func TestApplyValueFlowsCopiesUsageBackToSources(t *testing.T) {
	usage := NewUsage()
	usage.record(referenceForProperties("imported", "message"), openValue)
	flows := []ValueFlow{
		{Source: []string{"worker", "exports", "settings"}, Destination: []string{"imported"}},
		{Source: []string{"global"}, Destination: []string{"worker", "global"}},
	}

	if err := ApplyValueFlows(usage, flows); err != nil {
		t.Fatal(err)
	}
	message := usage.Properties["worker"].Properties["exports"].Properties["settings"].Properties["message"]
	if message == nil || !message.Open {
		t.Fatalf("import source usage = %#v", message)
	}
}

func TestApplyValueFlowsCopiesUsageToDestinations(t *testing.T) {
	usage := NewUsage()
	usage.record(referenceForProperties("global", "region"), readValue)
	flows := []ValueFlow{
		{Source: []string{"global"}, Destination: []string{"worker", "global"}},
	}

	if err := ApplyValueFlows(usage, flows); err != nil {
		t.Fatal(err)
	}
	region := usage.Properties["worker"].Properties["global"].Properties["region"]
	if region == nil || !region.Read {
		t.Fatalf("flow destination usage = %#v", region)
	}
}

func TestApplyValueFlowsHandlesCycles(t *testing.T) {
	usage := NewUsage()
	usage.MarkPath("example/Chart.yaml", "left", "value")
	flows := []ValueFlow{
		{Source: []string{"right"}, Destination: []string{"left"}},
		{Source: []string{"left"}, Destination: []string{"right"}},
	}

	if err := ApplyValueFlows(usage, flows); err != nil {
		t.Fatal(err)
	}
	if right := usage.Properties["right"].Properties["value"]; right == nil || !right.Read {
		t.Fatalf("cycle did not propagate usage: %#v", usage)
	}
}

func TestApplyValueFlowsPreservesEveryUsageMode(t *testing.T) {
	usage := NewUsage()
	usage.Properties["source"] = &Usage{
		Read: true, Exact: true, AllowUnknown: true, Open: true, Iterated: true,
	}
	flows := []ValueFlow{{
		Source: []string{"source"}, Destination: []string{"destination"},
	}}

	if err := ApplyValueFlows(usage, flows); err != nil {
		t.Fatal(err)
	}
	destination := usage.Properties["destination"]
	if destination == nil || !destination.Read || !destination.Exact ||
		!destination.AllowUnknown || !destination.Open || !destination.Iterated {
		t.Fatalf("flow destination usage = %#v", destination)
	}
}

func TestApplyValueFlowsCopiesSources(t *testing.T) {
	usage := NewUsage()
	usage.record(referenceForProperties("global", "region"), readValue, "example/templates/configmap.yaml")
	flows := []ValueFlow{
		{Source: []string{"global"}, Destination: []string{"worker", "global"}},
	}

	if err := ApplyValueFlows(usage, flows); err != nil {
		t.Fatal(err)
	}
	region := usage.Properties["worker"].Properties["global"].Properties["region"]
	if region == nil {
		t.Fatal("flow destination is missing")
	}
	if len(region.Sources) != 1 || region.Sources[0] != "example/templates/configmap.yaml" {
		t.Fatalf("flow destination sources = %#v", region.Sources)
	}
}

func TestAddSourceKeepsNamesSortedAndUnique(t *testing.T) {
	usage := NewUsage()
	usage.AddSource("b.yaml")
	usage.AddSource("a.yaml")
	usage.AddSource("b.yaml")
	usage.AddSource("")

	want := []string{"a.yaml", "b.yaml"}
	if len(usage.Sources) != len(want) {
		t.Fatalf("sources = %#v", usage.Sources)
	}
	for index, name := range want {
		if usage.Sources[index] != name {
			t.Fatalf("sources = %#v", usage.Sources)
		}
	}
}
