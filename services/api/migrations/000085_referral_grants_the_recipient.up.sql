-- A referral grants sight to the department it was sent to, and no wider.
--
-- 000083 matched a referral against the receiving officer's whole force
-- family. That is right for the ordinary boundary — a traffic sergeant should
-- see the station's record of the accident they attended — but wrong for a
-- referral, and wrong in the direction that matters.
--
-- CID is a wing of West Bengal Police, so under the family rule a case
-- referred to CID became visible to every West Bengal Police officer. CID
-- exists to take work away from the local force, often because the local force
-- is too close to it. Exposing a CID referral to the whole of West Bengal
-- Police defeats the referral.
--
-- Observed rather than reasoned about: a West Bengal Police station officer
-- was found holding sight of an FIR that Kolkata Police had referred to CID
-- and CID alone.
--
-- A referral now grants sight to the exact department named on it. Kolkata
-- Police referring to CID gives CID the record; it gives West Bengal Police
-- nothing.

CREATE OR REPLACE FUNCTION record_referred_to_force(p_type TEXT, p_record UUID, p_force UUID)
RETURNS BOOLEAN AS $$
    SELECT EXISTS (
        SELECT 1 FROM case_referrals r
         WHERE r.record_type = p_type
           AND r.record_id = p_record
           AND r.status = 'ACCEPTED'
           AND r.to_force_id = p_force
    )
$$ LANGUAGE sql STABLE;

COMMENT ON FUNCTION record_referred_to_force(TEXT, UUID, UUID) IS
    'Has this record been referred to this exact department and accepted by it? Deliberately not the department''s family: a case referred to CID is CID''s, not all of West Bengal Police''s.';
