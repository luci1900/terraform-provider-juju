# QA

After you have bootstrapped a controller, you can run a test:

```shell
TF_ACC=1 go test -v ./tests/temporal/...
```

This will run the plan, wait for the Juju application to be avaiable, then destroy everything.
