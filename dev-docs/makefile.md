# Makefile

The Makefile `tasks.mak` is a Makefile for the project that is used to quickly perform tasks or combinations of tasks that make development more convenient. All building is still done in the Dockerfile, and `make` is not going to be our build system, but this file can be useful for common things like testing and using tools which used to require scripts or aliases to long commands.

### Why is it called `tasks.mak` instead of `Makefile`?

We are unable to use the actual `Makefile` name in this repo; because we build the Ops Agent into a deb package, `debhelper` tries to autodetect `make` as the build system in most of its `dh_auto` steps. We do not want `debhelper` to pick up the Makfile and try picking up different targets to build and test it.

## Usage

For convenience it helps to make a symlink from `tasks.mak` to `Makefile`. There is a target to do this, to run it:
```
make -f tasks.mak makefile_symlink
```

Now you can call the targets simply with the `make` command without needing to specify the `tasks.mak` file. The file `Makefile` is gitignored, since we don't want to push it to the repo for the reasons stated in the previous section. 

To run something from the Makefile, you will need to give `make` the `-f` (file) flag with a path to `tasks.mak`. For example, to run the `test` target:
```
make test
```

If you want to run one of the tool targets (i.e. `addlicense` or `yaml_format`), you'll first need to run the `install_tools` target:
```
make install_tools
```

### Precommit

There are two targets, `precommit` and `precommit_update`, that are meant to be run before making commits. The `precommit` target will run specific tests and checks to make sure the changes are good to commit, and `precommit_update` will actually perform updates (i.e. yaml formatting and golden file updates) that will prepare your code for commit. Currently these targets don't run automatically as precommit hooks, to give developers the option of updating or simply checking in their precommit, however this is subject to change.

## When to add a new target

A new target should be added to `tasks.mak` if:
* There's a new package with Unit Tests
* There is something that developers may need to run often, especially if it's a complicated command
* There are new tasks to run in CI
* There is a new development tool to be installed and used 

## Disclaimer

`tasks.mak` is provided without any guarantees or warranty on its targets. It is meant purely for developer convenience, and it is advised not to make any dependency on the targets since they are subject to change at any time.