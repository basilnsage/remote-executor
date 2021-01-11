## Remote Executor
This is a small program designed to wrap remote command execution.

The premise is to wrap executing a single command against a list of hosts with each host run concurrently.

The API is broken out into a sublibrary while the root of the project contains a script with one possible implementation.

### Tuning with flags
The program can be tuned with the following flags:
- --concurrency \<number\>
    - default 100; the number of hosts to concurrently connect to
- --check-hostkey=true/false
    - default true; enable/disable remote hostkey checking
- --parser \<string\>
    - default '^([^\s]*)\b': regex to parse each line of the host list with
    - note: the regex must contain a capture group or no remote hosts will be identified
- --user <remote user>
    - default: $USER
- --private-key </path/to/private/key>
    - default: $HOME/.ssh/id_rsa
- --known-hosts </path/to/known_hosts/file>
    - default: %HOME/.ssh/known_hosts
    
### Running
CLI usage:

`./remote-executor [...options] path_to_host_list cmd_to_run`
