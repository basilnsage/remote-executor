## Remote Executor
This is a small program designed to wrap remote command execution.

The premise is to wrap executing a single command against a list of hosts with each host run concurrently.

The API is broken out into a sublibrary while the root of the project contains a script with one possible implementation.

### Tuning with flags
The program can be tuned with the following flags:
- --concurrency=\<number\>
    - default 100; the number of hosts to concurrently connect to
- --check-hostkey
    - default false; specify to enable host key checking (more secure)
- --parser=\<string\>
    - default '^([^\s]*)\b': regex to parse each line of the host list with
    - note: the regex must contain a capture group or no remote hosts will be identified
- --user=<remote user>
    - default $USER
- --private-key=</path/to/private/key>
    - default $HOME/.ssh/id_rsa
- --known-hosts=</path/to/known_hosts/file>
    - default %HOME/.ssh/known_hosts
- --summarize
    - default false; specify to print a summary of failed hosts at the end
    - note: displays failed hosts at the end of the run
    
### Running
*Note*: quotes required for commands consisting of more than 1 word

CLI usage:

`./remote-executor [...options] path_to_host_list "command to run"`
