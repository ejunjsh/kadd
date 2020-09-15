# kadd

a command to add container into a running pod on k8s

## precondition

install a docker desktop and enable the k8s

## getting started

### build image

    docker build -t ejunjsh/kadd-controller:1.0 .
    
### build command

    cd cmd/kadd
    go install
    
### run command

    kadd vpnkit-controller bash -n kube-system --image nginx
    # above command means you add a container whose image is nginx and that container run in the vpnkit-controller pod whose namespace is kube-system
