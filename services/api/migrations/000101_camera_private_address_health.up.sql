-- A camera on a station LAN is not broken just because this server cannot see it.
--
-- Phase 03's reachability check opens a TCP connection from the API to the
-- camera's stream host. That was written for the MDC box, which sits on the
-- same network as the cameras. On the hosted tier the API is nowhere near
-- them: a DVR at 192.168.100.64 is unroutable from Vercel no matter how
-- healthily it is streaming, so every real camera reads UNREACHABLE — the
-- screen says the camera is down when what is down is the route.
--
-- The register gains a fifth state, NOT_ROUTABLE, for a failed check against a
-- private address. It says what was actually established: nothing, because
-- this server has no path there. Liveness for such a camera comes from the
-- Edge Agent's segments arriving (ONLINE / CONNECTING / STOPPED), which is the
-- truer signal anyway — "reachable" only ever meant the port answered.
--
-- On the edge box nothing changes: the dial succeeds and the camera reads
-- REACHABLE as before.

CREATE OR REPLACE FUNCTION is_private_host(host TEXT) RETURNS BOOLEAN AS $$
DECLARE
    addr inet;
BEGIN
    IF host IS NULL OR btrim(host) = '' THEN
        RETURN FALSE;
    END IF;
    BEGIN
        addr := btrim(host)::inet;
    EXCEPTION WHEN others THEN
        -- A hostname, not a literal address. We cannot tell where it points
        -- without resolving it, and a health check is not the place to do
        -- that, so it is treated as routable and the dial decides.
        RETURN FALSE;
    END;
    RETURN addr <<= '10.0.0.0/8'::inet
        OR addr <<= '172.16.0.0/12'::inet
        OR addr <<= '192.168.0.0/16'::inet
        OR addr <<= '127.0.0.0/8'::inet
        OR addr <<= '169.254.0.0/16'::inet
        OR addr <<= 'fc00::/7'::inet
        OR addr <<= 'fe80::/10'::inet
        OR addr <<= '::1/128'::inet;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMENT ON FUNCTION is_private_host(TEXT) IS
    'True when the host is a literal address on a private, loopback or link-local range. Used to tell a camera this server cannot route to from one that is genuinely down.';
