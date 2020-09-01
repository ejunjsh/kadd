# kadd

a command to add container into a running pod

## getting started

### build image

    docker build -t ejunjsh/kadd-controller:1.0 .
    
### build command

    go install
    
### run command

    kadd vpnkit-controller bash -n kube-system --image nginx
