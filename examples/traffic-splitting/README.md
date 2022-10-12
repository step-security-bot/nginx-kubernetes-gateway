# Example

In this example we will deploy NGINX Kubernetes Gateway and configure traffic splitting for a simple cafe application.
We will use `HTTPRoute` resources to split traffic between two versions of the application -- `coffee-v1` and `coffee-v2`.

## Running the Example

## 1. Deploy NGINX Kubernetes Gateway

1. Follow the [installation instructions](/docs/installation.md) to deploy NGINX Gateway.

1. Save the public IP address of NGINX Kubernetes Gateway into a shell variable:

   ```
   GW_IP=XXX.YYY.ZZZ.III
   ```

1. Save the port of NGINX Kubernetes Gateway:

   ```
   GW_PORT=<port number>
   ```

## 2. Deploy the Coffee Application

1. Create the Cafe Deployments and Services:

   ```
   kubectl apply -f cafe.yaml
   ```

1. Check that the Pods are running in the `default` namespace:

   ```
   kubectl -n default get pods
   NAME                         READY   STATUS    RESTARTS   AGE
   coffee-v1-7c57c576b-rfjsh    1/1     Running   0          21m
   coffee-v2-698f66dc46-vcb6r   1/1     Running   0          21m
   ```

## 3. Configure Routing

1. Create the `Gateway`:

   ```
   kubectl apply -f gateway.yaml
   ```

1. Create the `HTTPRoute` resources:

   ```
   kubectl apply -f cafe-route.yaml
   ```
   
This `HTTPRoute` resource defines a route for the path `/coffee` that sends 80% of the requests to `coffee-v1` and 20% to `coffee-v2`.

## 4. Test the Application

To access the application, we will use `curl` to send requests to `/coffee`:

```
curl --resolve cafe.example.com:$GW_PORT:$GW_IP http://cafe.example.com:$GW_PORT/coffee
```

80% of the responses will come from `coffee-v1`:

```
Server address: 10.12.0.18:80
Server name: coffee-v1-7c57c576b-rfjsh
```

20% of the responses will come from `coffee-v2`:

```
Server address: 10.12.0.19:80
Server name: coffee-v2-698f66dc46-vcb6r
```
