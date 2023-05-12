# argocd-vault-replacer
A  plugin for [ArgoCD lovely plugin](https://github.com/crumbhole/argocd-lovely-plugin) to replace placeholders in Kubernetes manifests with secrets stored in [Hashicorp Vault](https://www.vaultproject.io/). The binary will scan the current directory recursively for any .yaml (or .yml if you're so inclined) files, or take yaml from stdin, and attempt to replace strings of the form `<secret:/store/data/path~key>` with those obtained from a Vault kv2 store.

If you use it as the reader in a unix pipe, it will instead read from stdin. In this scenario it can post-process the output of another tool, such as Kustomize or Helm.

This plugin used to be available as a direct plugin to ArgoCD, but has not been adapted for ArgoCD 2.7's need for running as a sidecar. If this need is one you have, please raise an issue or ideally a PR. The authors now only use this through lovely plugin.

Note: This and previous versions of this plugin only talk to vault, and hence <secret:...> can also be specified as <vault:...>. Future plans may include other secret providers.

<img src="assets/images/argocd-vault-replacer-diagram.png">

## Why?
- Allows you to invest in Git Ops without compromising secret security.
  - Configuration goes into Git.
  - Secrets go into Vault.
- yaml-agnostic. Supports any Kubernetes resource type as long as it can be expressed in .yaml (or .yml).
  - Also supports Argo CD-managed Kustomize and Helm charts
- Native Vault-Kubernetes authentication means you don't have to renew tokens or store/passthrough approle role-ids and secret-ids.

#Installing

## As a lovely plugin

Install [ArgoCD lovely plugin](https://github.com/crumbhole/argocd-lovely-plugin) using the ghcr.io/crumbhole/argocd-lovely-plugin-cmp-vault image. Setup your vault-replacer environment variables in that sidecar.

## Installing as an Argo CD Plugin (deprecated)
You can use [our Kustomization example](https://github.com/crumbhole/argocd-vault-replacer/tree/main/examples/kustomize/argocd) to install Argo CD and to bootstrap the installation of the plugin at the same time. However the steps below will detail what is required should you wish to do things more manually. The Vault authentication setup cannot be done with Kustomize and must be done manually.

## Vault Kubernetes Authentication
You will need to set up the Vault Kubernetes authentication method for your cluster.

You will need to create a service account. In this example, our service account will be called 'argocd'. Our example creates the serviceAccount in the argocd namespace:

```YAML
apiVersion: v1
kind: ServiceAccount
metadata:
  name: argocd
  namespace: argocd
```

For Kubernetes [version 1.24](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.24.md#urgent-upgrade-notes) and newer there is no automatic service account token generated. This must be generated as a secret and the service account must refer to said secret:

```YAML
apiVersion: v1
kind: ServiceAccount
metadata:
  namespace: argocd
  name: argocd
secrets:
- name: argocd-sa-token
---
apiVersion: v1
kind: Secret
metadata:
  namespace: argocd
  name: argocd-sa-token
  annotations:
    kubernetes.io/service-account.name: argocd
type: kubernetes.io/service-account-token
```

You will need to tell Vault about this Service Account and what policy/policies it maps to:

```
vault write auth/kubernetes/role/argocd \
        bound_service_account_names=argocd \
        bound_service_account_namespaces=argocd \
        policies=argocd \
        ttl=1h
```
This is better documented by Hashicorp themselves, do please refer to [their documentation](https://www.vaultproject.io/docs/auth/kubernetes).

Lastly, you will need to modify the argocd-repo-server deployment to use your new serviceAccount, and to allow the serviceAccountToken to automount when the pod starts up. You must patch the deployment with:
```YAML
apiVersion: apps/v1
kind: Deployment
metadata:
  name: patch-serviceAccount
spec:
  template:
    spec:
      serviceAccount: argocd
      automountServiceAccountToken: true
```
## Plugin Installation
In order to install the plugin into Argo CD, you can either build your own Argo CD image with the plugin already inside, or make use of an Init Container to pull the binary. Argo CD's documentation provides further information how to do this: https://argoproj.github.io/argo-cd/operator-manual/custom_tools/

We offer a pre-built init container that moves the binary into /custom-tools on startup, so an init container manifest will look something like this:
```YAML
containers:
- name: argocd-repo-server
  volumeMounts:
  - name: custom-tools
    mountPath: /usr/local/bin/argocd-vault-replacer
    subPath: argocd-vault-replacer
  envFrom:
    - secretRef:
        name: argocd-vault-replacer-credentials
volumes:
- name: custom-tools
  emptyDir: {}
initContainers:
- name: argocd-vault-replacer-install
  image: ghcr.io/crumbhole/argocd-vault-replacer
  imagePullPolicy: Always
  volumeMounts:
    - mountPath: /custom-tools
      name: custom-tools
```
The above references a Kubernetes secret called "argocd-vault-replacer-credentials". We use this to pass through the mandatory ARGOCD_ENV_VAULT_ADDR environment variable. We could also use it to pass through optional variables too
```YAML
apiVersion: v1
data:
  ARGOCD_ENV_VAULT_ADDR: aHR0cHM6Ly92YXVsdC5leGFtcGxlLmJpeg==
kind: Secret
metadata:
  name: argocd-vault-replacer-credentials
  namespace: argocd
type: Opaque
```

Environment Variables:

| Environment Variable Name | Purpose                                                                                                                               | Example                           | Mandatory? |
|-------------------------- |-------------------------------------------------------------------------------------------------------------------------------------- |---------------------------------- |----------- |
| ARGOCD_ENV_VAULT_ADDR                | Provides argocd-vault-replacer with the URL to your Hashicorp Vault instance.                                                         | https://vault.examplecompany.biz  | Y
| ARGOCD_ENV_VAULT_TOKEN               | A valid Vault authentication token. This should only be used for debugging. This won't work inside kubernetes if you have a service account token available, as the tool considers a service account token that fails to authenticate a complete failure. You'll have to run a pod without a service account if you want to use this. The token cannot be renewed by the tool so if it expires, the tool will stop.                                                          | s.LLijB190n3c8s4fiSuvTdVNM        | N
| ARGOCD_ENV_VAULT_ROLE                | The name of the role for the VAULT_TOKEN. This defaults to 'argocd'.                                                                  | argocd-role                       | N
| ARGOCD_ENV_VAULT_AUTH_PATH           | Determines the authorization path for Kubernetes authentication. This defaults to 'kubernetes' so will probably not need configuring. | kubernetes                        | N

Before Argo CD 2.4 these did not need to be prefixed with ARGOCD_ENV_, and the current version will accept either type, with precedence given to the ARGOCD_ENV_ version.

If you are passing the configuration in as application environment variables in Argo CD 2.4 or higher you must not put the ARGOCD_ENV_ prefix on them, as Argo CD does that for you.

## Plugin Configuration
After installing the plugin into the /custom-tools/ directory, you need to register it inside the Argo CD config. Declaratively, you can add this to your argocd-cm configmap file:

```YAML
configManagementPlugins: |-
  - name: argocd-vault-replacer
    generate:
      command: ["argocd-vault-replacer"]
  - name: kustomize-argocd-vault-replacer
    generate:
      command: ["sh", "-c"]
      args: ["kustomize build . | argocd-vault-replacer"]
  - name: helm-argocd-vault-replacer
    init:
      command: ["/bin/sh", "-c"]
      args: ["helm dependency build"]
    generate:
      command: [sh, -c]
      args: ["helm template -n $ARGOCD_APP_NAMESPACE $ARGOCD_APP_NAME . | argocd-vault-replacer"]
```

This is documented further in Argo CD's documentation: https://argoproj.github.io/argo-cd/user-guide/config-management-plugins/

* argo-vault-replacer as a plugin will only work on a directory full of straight .yaml files.
* kustomize-argo-vault-replacer as a plugin will take the output of kustomize and then do vault-replacement on those files. Note: This won't allow you to use the argo application kustomization options, it just runs a straight kustomize.
* helm-argo-vault-replacer as a plugin will take the output of Helm and then do vault-replacement on those files. Note: This won't allow you to use the argo application Helm options, it just runs a straight Helm with the default argo name.
## Testing

Create a test yaml file that will be used to pull a secret from Vault. The below will look in Vault for /path/to/your/secret and will return the key 'secretkey', it will then base64 encode that value. As we are using a Vault kv2 store, we must include ..`/data/`.. in our path:

```YAML
apiVersion: v1
kind: Secret
metadata:
  name: argocd-vault-replacer-secret
data:
  sample-secret: <secret:path/data/to/your/secret~secretkey|base64>
type: Opaque
```
In this example, we pushed the above to `https://github.com/replace-me/argocd-vault-replacer-test/argocd-vault-replacer-secret.yaml`

We then deploy this as an Argo CD application, making sure we tell the application to use the argocd-vault-replacer plugin:

```YAML
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: argocd-vault-replacer-test
spec:
  destination:
    server: 'https://kubernetes.default.svc'
    namespace: argocd-vault-replacer-test
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
  source:
    repoURL: 'https://github.com/replace-me'
    path: argocd-vault-replacer-test
    plugin:
      name: argocd-vault-replacer
    targetRevision: HEAD
```

There are further examples to use for testing in the [examples directory](https://github.com/crumbhole/argocd-vault-replacer/tree/main/examples/).
## A deep-dive on authentication

The tool only has two methods of authenticating with Vault:
* Using kubernetes authentication method https://github.com/hashicorp/vault/blob/master/website/content/docs/auth/kubernetes.mdx
* Using a token, which is only intended for debugging

Both methods expect the environment variable ARGOCD_ENV_VAULT_ADDR to be set.

It will attempt to use kubernetes authentication through an appropriate service account first, and complain if that doesn't work. It will then use VAULT_TOKEN which should be a valid token. This tool has no way of renewing a token or obtaining one other than through a kubernetes service account.

To use the kubernetes service account your pod should be running with the appropriate service account, and will try to obtain the JWT token from /var/run/secrets/kubernetes.io/serviceaccount/token which is the default location.

It will use the environment variable ARGOCD_ENV_VAULT_ROLE as the name of the role for that token, defaulting to "argocd".
It will use the environment variable ARGOCD_ENV_VAULT_AUTH_PATH to determine the authorization path for kubernetes authentication. This defaults in this tool and in vault to "kubernetes" so will probably not need configuring.

The vault authentication token that the tool gets will not be cached, nor will it be renewed. It is expected that the token will last for the length of the tool's invokation, which is usually a reasonable assumption in the use case for which it was designed.

## Valid vault paths

Currently the only valid 'URL style' to a path is

`<secret:/store/data/path~key~key~key|modifier|modifier>`

You must put ..`/data/`.. into the path. If your path or key contains `~`, `<`, `>` or `|` you must URL escape it. If your path or key has one or more leading or trailing spaces or tabs you must URL escape them you weirdo.

Any base64 encoded strings will be decoded and subsitution will happen within them (and end up being base64 encoded afterwards). These base64 strings require some form of whitespace (any non-base64 valid character) around them in order to be detected.

## Modifiers

You can modify the resulting output with the following modifiers:

* base64: Will base64 encode the secret. Use for data: sections in kubernetes secrets.
* Other modifiers are documented in [docs/modifiers](docs/modifiers.md)

## Rotating secrets in Vault
Currently, because Argo CD cannot monitor Vault for changes, when you change a secret in Vault, Argo CD will not automatically update your Kubernetes resources with the new value. You will have to either push a change to git, use the Hard Refresh option in Argo CD, or force Argo CD to heal by deleting the Kubernetes resource in question.

## Development

This project only builds with go 1.18 (and presumably later when later happens).

You can build in the docker container, or look at the [Makefile] to build and test conventionally.

Please add tests for any new/changed functionality.
