**CHANGE DESCRIPTION**
---
**Problem:**
*Short explanation of the problem.*

**Cause:**
*Short explanation of the root cause of the issue if applicable.*

**Solution:**
*Short explanation of the solution we are providing with this PR.*

**CHECKLIST**
---
**Jira**
- [ ] Is the Jira ticket created and referenced properly?
- [ ] Does the Jira ticket have the proper statuses for documentation (`Needs Doc`) and QA (`Needs QA`)?
- [ ] Does the Jira ticket link to the proper milestone (Fix Version field)?

**Tests**
- [ ] Are E2E tests passing?
- [ ] Are unit tests passing?
- [ ] Are linting tests passing?
- [ ] Is an E2E test/test case added for the new feature/change?
- [ ] Are unit tests added where appropriate?
- [ ] Are OpenShift compare files changed for E2E tests (`compare/*-oc.yml`)?

**Config/Logging/Testability**
- [ ] Are all needed new/changed options added to default YAML files?
- [ ] Are the manifests (crd/bundle) regenerated if needed?
- [ ] Did we add proper logging messages for operator actions?
- [ ] Did we ensure compatibility with the previous version or cluster upgrade process?
- [ ] Does the change support oldest and newest supported PXC version?
- [ ] Does the change support oldest and newest supported Kubernetes version?
