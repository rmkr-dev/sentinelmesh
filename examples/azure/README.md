# Azure example

Use the profile that matches the bill you are willing to pay.

```bash
cd infra/terraform
terraform init
terraform plan -var-file=profiles/azure-minimal.tfvars -var=subscription_id="$ARM_SUBSCRIPTION_ID"
```

The variable file is `infra/terraform/profiles/azure-minimal.tfvars`. Production is the other file in that directory. Read `docs/deployment/azure.md` before apply.
