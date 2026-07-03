
  function shallow_merge(record, parsedRecord)
    -- If no exiting record exists
    if (record == nil) then 
        return parsedRecord
    end
    
    for k, v in pairs(parsedRecord) do
        record[k] = v
    end

    return record
end

function merge(record, parsedRecord)
    -- If no exiting record exists
    if record == nil then 
        return parsedRecord
    end
    
    -- Potentially overwrite or merge the original records.
    for k, v in pairs(parsedRecord) do
        -- If there is no conflict
        if k == "logging.googleapis.com/logName" then 
            -- Ignore the parsed payload since the logName is controlled
            -- by the OpsAgent.
        elseif k == "logging.googleapis.com/labels" then 
            -- LogEntry.labels are basically a map[string]string and so only require a
            -- shallow merge (one level deep merge).
            record[k] = shallow_merge(record[k], v)
        else
            record[k] = v
        end
    end

    return record
end

function parser_merge_record(tag, timestamp, record)
    originalPayload = record["logging.googleapis.com/__tmp"]
    if originalPayload == nil then
        return 0, timestamp, record
    end
    
    -- Remove original payload
    record["logging.googleapis.com/__tmp"] = nil
    record = merge(originalPayload, record)
    return 2, timestamp, record
end
