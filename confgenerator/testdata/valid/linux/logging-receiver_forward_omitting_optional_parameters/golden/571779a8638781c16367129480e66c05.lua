
function split (inputstr, sep)
  if sep == nil then
    sep = "%s"
  end
  local t={}
  for str in string.gmatch(inputstr, "([^"..sep.."]+)") do
    table.insert(t, str)
  end
  return t
end

function add_log_name(tag, timestamp, record)
  -- Split the tag by . to separate the pipeline_id from the rest
  local split_tag = split(tag, '.')
  
  -- The tag is formatted like the following:
  -- <hash_string>.<pipeline_id>.<receiver_id>.<existing_tag>
  --
  -- We can assert that the hash_string, pipeline_id, receiver_id do not
  -- contain the "." delimiter.
  -- We append the existing_tag to the LogName field in the record.
  local receiver_uuid = table.remove(split_tag, 1)
  local pipeline_id = table.remove(split_tag, 1)
  local receiver_id = table.remove(split_tag, 1)
  local existing_tag = table.concat(split_tag, ".")

  -- Replace the record with one with the log name
  record["logging.googleapis.com/logName"] = record["logging.googleapis.com/logName"] .. "." .. existing_tag
  
  -- Use code 2 to replace the original record but keep the original timestamp
  return 2, timestamp, record
end
