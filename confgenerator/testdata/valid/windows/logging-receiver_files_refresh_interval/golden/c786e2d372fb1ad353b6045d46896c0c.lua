
function process(tag, timestamp, record)
    io.write(tag, ": [2] check message=", record["Message"], "\n");
    return 0, timestamp, record
end