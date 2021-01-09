## Remote Executor
This is a small program designed to wrap remote command execution.

The premise is to wrap executing a single command against a list of hosts with each host run concurrently.

### Tuning with flags
The program can be tuned with the following flags:
- --concurrency=\<number\>
    - default 100; the number of hosts to concurrently connect to
- --check-hostkey=true/false
    - default true; enable/disable remote hostkey checking
- --parser=\<string\>
    - default '^\(\d{1-3}\.\d{1-3}\.\d{1-3\}\.\d{1-3}/)': regex to parse each line of the host list with
    - note: the regex must contain a capture group or no remote hosts will be identified
    
### Running
CLI usage:

`./remote-executor [--concurrency=100] [--check-hostkey=true] [--parser='foobar'] path_to_host_list cmd_to_run`
