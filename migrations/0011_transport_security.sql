-- Whether the agent's control-channel connection was actually made over
-- wss:// (i.e. its configured server_url is https://), reported honestly
-- by the agent itself at every handshake (protocol.HelloPayload.Secure)
-- — see internal/config.Advisory for the server-config-based equivalent
-- check. NULL means "unknown" — either the device hasn't connected since
-- this column was added, or is running a pre-0.2.1 agent that never
-- reported it.
ALTER TABLE devices ADD COLUMN transport_secure boolean;
