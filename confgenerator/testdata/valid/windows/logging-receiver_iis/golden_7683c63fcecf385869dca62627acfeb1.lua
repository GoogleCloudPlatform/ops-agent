
	function iis_merge_fields(tag, timestamp, record)
	  record["http_request_serverIp"] = table.concat({record["http_request_serverIp"], ":", record["s_port"]})
	  if (record["cs_uri_query"] == nil or record["cs_uri_query"] == '') then
		record["http_request_requestUrl"] = record["cs_uri_stem"]
	  else
		record["http_request_requestUrl"] = table.concat({record["cs_uri_stem"], "?", record["cs_uri_query"]})
	  end
	  return 2, timestamp, record
	  
	end
	