terraform {
  required_providers {
    applicationmanagement = {
      source  = "novus/applicationmanagement"
      version = ">= 0.1.14"
    }
  }
}

/*
provider "applicationmanagement" {
  oauth {
    client_id      = jsondecode(data.aws_secretsmanager_secret_version.management_plane_application_management_client.secret_string)["oauth_client_id"]
    client_secret  = jsondecode(data.aws_secretsmanager_secret_version.management_plane_application_management_client.secret_string)["oauth_client_secret"]
    scope          = jsondecode(data.aws_secretsmanager_secret_version.management_plane_application_management_client.secret_string)["oauth_scope"]
    token_endpoint = jsondecode(data.aws_secretsmanager_secret_version.management_plane_application_management_client.secret_string)["oauth_token_endpoint"]
    resource       = jsondecode(data.aws_secretsmanager_secret_version.management_plane_application_management_client.secret_string)["oauth_resource"]
  }
}

provider config

provider_installation {
    direct {
      include = ["*\/\*"]
}
network_mirror {
url = "https://artifactory.novus.legogroup.io/artifactory/api/terraform/terraform-providers/providers/"
}
}
*/

resource "applicationmanagement_singlesignon_client" "test-app" {
  name                  = "tf-reconciler-test"
  description           = "Test client remove at will"
  environment           = "Production"
  leanix_application_id = "APP-02358"
  type                  = "oidc"
  client_type           = "web"
  identifier_id         = "api://tf-reconciler-test"
#   redirect_uris         = ["https://tf-reconciler-test.example"]
}
