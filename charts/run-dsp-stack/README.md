# RUN-DSP-stack

This is a parent chart to deploy a RUN-DSP setup with

- [RUN-DSP](https://codeberg.org/go-dataspace/run-dsp)
- [reference-contract-service](https://codeberg.org/go-dataspace/reference-contract-service)
- [reference-authn-service](https://codeberg.org/go-dataspace/reference-authn-service)
- [rdsp-s3](https://codeberg.org/go-dataspace/rdsp-s3/)

## Requirements

For this deployment you will need to have

- Access to an S3 compatible service
- A k8s Secret for mTLS certificate/key
- A k8s ConfigMap containing CA certificate

## Example setup

First let's setup a namespace:

`kubectl create namespace run-dsp-testing`

Then if we are using cert-manager we can first create a cluster issuer:

`kubectl apply -f example/cluster_issuer.yaml `

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: ca-issuer-helmtest
spec:
    selfSigned: {}
```

And then a certificate:

`kubectl apply -n run-dsp-testing -f example/certificate.yaml `

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: helmtest-cert
spec:
  dnsNames:
  - stacktest-rdsp-s3.run-dsp-testing
  ipAddresses:
  - "127.0.0.1"
    isCA: false
  issuerRef:
    kind: ClusterIssuer
    name: ca-issuer-helmtest
  secretName: helmtest-cert-secret
  usages:
  - client auth
  - server auth
```
Now we can extract the secret and put it in a ConfigMap:

`kubectl create -n run-dsp-testing configmap helmtest-ca-configmap --from-literal="ca.crt=$(kubectl get -n run-dsp-testing secret helmtest-cert-secret -o jsonpath="{.data.ca\.crt}" | base64 -d)"`

And then we can apply a common configuration for the stack:

```yaml
# Global section used between the RUN-DSP family helm charts.
global:
    # TLS settings.
    tls:
      # Disables TLS.
      insecure: false
      verifyClientCerts: true

      # TLS Secret that contains certificate/key pair to connect services,
      # None used if left empty
      runDspClientCertSecret: "helmtest-cert-secret"
      controlServiceCertSecret: "helmtest-cert-secret"
      authnServiceCertSecret: "helmtest-cert-secret"
      providerServiceCertSecret: "helmtest-cert-secret"
      contractServiceCertSecret: "helmtest-cert-secret"
      controlClientCertSecret: "helmtest-cert-secret"

      # CA certificate.
      caCert:
        # The config map containing the CA cert.
        configMap: "helmtest-ca-configmap"
        # The key for the config map.
        key: "ca.crt"

rdsp-s3:
  s3:
    baseURL: <S3 API endpoint>
    bucket: <your bucket>
    credentials:
      accessKey: <your access key>
      secretKey: <your secret key>
```

**Note:** You need to edit `stack_values.yaml` with your S3 config before applying.

First add the chart repository :

`helm repo add go-dataspace https://helm.go-dataspace.eu/charts`

And then install the chart:

`helm install -n run-dsp-testing --debug stacktest go-dataspace/run-dsp-stack -f example/stack_values.yaml`


`helm install --debug billybokhylla go-dataspace/run-dsp-stack -f example/stack_values.yaml`

Using `kubectl get -n run-dsp-testing all` we should get something similar to

```
NAME                                                       READY   STATUS    RESTARTS        AGE
pod/stacktest-rdsp-s3-0                                    1/1     Running   0               2m27s
pod/stacktest-reference-authn-service-5fdb58df56-w4xts     1/1     Running   0               2m27s
pod/stacktest-reference-contract-service-5f6bdb859-68dsl   1/1     Running   0               2m27s
pod/stacktest-run-dsp-0                                    1/1     Running   1 (2m26s ago)   2m27s

NAME                                           TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)                      AGE
service/stacktest-rdsp-s3                      ClusterIP   10.43.175.113   <none>        9090/TCP,9091/TCP,8082/TCP   2m27s
service/stacktest-reference-authn-service      ClusterIP   10.43.111.47    <none>        9093/TCP,2112/TCP            2m27s
service/stacktest-reference-contract-service   ClusterIP   10.43.19.168    <none>        9092/TCP,2112/TCP            2m27s
service/stacktest-run-dsp                      ClusterIP   10.43.51.86     <none>        8080/TCP,8081/TCP,8082/TCP   2m27s

NAME                                                   READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/stacktest-reference-authn-service      1/1     1            1           2m27s
deployment.apps/stacktest-reference-contract-service   1/1     1            1           2m27s

NAME                                                             DESIRED   CURRENT   READY   AGE
replicaset.apps/stacktest-reference-authn-service-5fdb58df56     1         1         1       2m27s
replicaset.apps/stacktest-reference-contract-service-5f6bdb859   1         1         1       2m27s

NAME                                 READY   AGE
statefulset.apps/stacktest-rdsp-s3   1/1     2m27s
statefulset.apps/stacktest-run-dsp   1/1     2m27s
```

Now we can test that this works using the [dspace-cli](https://codeberg.org/go-dataspace/dspace-cli) client. For this we will need to retrieve some data from the certicate secret.

```bash
kubectl get -n run-dsp-testing secret helmtest-cert-secret -o jsonpath="{.data.ca\.crt}" | base64 -d > ca.crt
kubectl get -n run-dsp-testing secret helmtest-cert-secret -o jsonpath="{.data.tls\.crt}" | base64 -d > tls.crt
kubectl get -n run-dsp-testing secret helmtest-cert-secret -o jsonpath="{.data.tls\.key}" | base64 -d > tls.key
```

We need to port forward to the service:

`kubectl port-forward -n run-dsp-testing svc/stacktest-run-dsp 8081`

And we can fix up a config file for the cli tool using sed:

`sed "s|__PWD__|$(pwd)|" example/dspace-cli.toml.template > dspace-cli.toml`

Now we can run the cli tool:

`dspace client -c ./dspace-cli.toml getcatalog http://stacktest-run-dsp.run-dsp-testing:8080`

And if the bucket contains a `README.md`file we should get something similar to this output:

```
Using config file: ./dspace-cli.toml
 INFO  Fetching catalogue from http://stacktest-run-dsp.run-dsp-testing:8080
 INFO  Catalogue received
 ID                  urn:uuid:cdaa419c-f66b-46ca-a0f0-05d2cc52555b
Title               README.md
Access Methods  
Description (en)    Detected by provider
Keywords  
Creator  
Issued              2025-10-22T14:54:13Z
Modified            2025-10-22T14:54:13Z
License  
AccessRights  
Rights  
ByteSize            11264
MediaType           text/markdown
Format  
CompressFormat  
PackageFormat  
Checksum: Algorithm
Checksum: Value  
```
