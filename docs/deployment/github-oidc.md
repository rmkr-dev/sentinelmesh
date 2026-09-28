# GitHub OIDC

1. Create an Entra ID app registration. Do not create a client secret.
2. Add a federated credential for the GitHub repository:
   - Issuer: `https://token.actions.githubusercontent.com`
   - Subject for the production environment: `repo:<org>/<repo>:environment:production`
   - Subject for development: `repo:<org>/<repo>:environment:development`
3. Assign the app `Contributor` and a role that can write role assignments, scoped to the subscription or resource group you want this repo to manage.
4. Store `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, and `AZURE_SUBSCRIPTION_ID` as repository secrets. They are identifiers, not keys. The workflow still treats them as secrets so they are masked.
5. Create GitHub environments `production` and `development`. Require reviewers on `production`.
6. Set Actions variables `AKS_RESOURCE_GROUP` and `AKS_CLUSTER_NAME` before running `deploy-aks.yml`.

The workflows request `id-token: write` and `contents: read`. They do not echo tokens.
