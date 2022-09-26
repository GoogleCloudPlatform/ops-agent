
function parser_nest(tag, timestamp, record)
  local nestedRecord = {}
  local parseKey = "key_1"
  for k, v in pairs(record) do
      if k ~= parseKey then
          nestedRecord[k] = v
      end
  end

  local result = {}
  result[parseKey] = record[parseKey]
  result["logging.googleapis.com/__tmp"] = nestedRecord

  return 2, timestamp, result
end

