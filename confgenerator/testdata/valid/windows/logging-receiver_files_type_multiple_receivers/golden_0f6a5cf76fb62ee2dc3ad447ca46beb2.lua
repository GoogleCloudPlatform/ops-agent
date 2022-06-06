
  function parser_nest(tag, timestamp, record)
  record["logging.googleapis.com/__tmp"] = {}
  
  for k, v in pairs(record) do
      if k ~= "key_5" then
          if record["logging.googleapis.com/__tmp"] == nil then
              record["logging.googleapis.com/__tmp"] = {}
          end
          record["logging.googleapis.com/__tmp"][k] = v
          record[k] = nil
      end
  end

  return 2, timestamp, record
end
