
function process(tag, timestamp, record)
    io.write(tag, ": [0] message=", record["Message"], "\n");
    return 0, timestamp, record
end