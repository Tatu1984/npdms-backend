#!/usr/bin/env python3
"""
NPDMS Demo Data Seed Script
Generates realistic sample data for client demonstrations
"""

import psycopg2
from psycopg2.extras import execute_values
from datetime import datetime, timedelta
import random
import json
import os
from faker import Faker

fake = Faker('en_IN')  # Indian locale for realistic names/addresses

# Database connection - must be set via environment variable
DB_URL = os.getenv('DATABASE_URL')
if not DB_URL:
    print("ERROR: DATABASE_URL environment variable is required")
    print("Example: DATABASE_URL='postgres://user:****@localhost:5432/npdms' python seed-demo-data.py")
    exit(1)

# Sample data constants
CRIME_CATEGORIES = [
    'THEFT', 'ROBBERY', 'BURGLARY', 'ASSAULT', 'MURDER',
    'KIDNAPPING', 'FRAUD', 'CYBERCRIME', 'DRUG_OFFENSE', 'DOMESTIC_VIOLENCE'
]

IPC_SECTIONS = {
    'THEFT': ['379', '380', '381'],
    'ROBBERY': ['392', '393', '394'],
    'BURGLARY': ['456', '457', '458', '460'],
    'ASSAULT': ['323', '324', '325', '326'],
    'MURDER': ['302', '304', '307'],
    'KIDNAPPING': ['363', '364', '365', '366'],
    'FRAUD': ['406', '420', '467', '468', '471'],
    'CYBERCRIME': ['66C', '66D', '66E', '66F'],
    'DRUG_OFFENSE': ['8', '15', '20', '21', '22'],
    'DOMESTIC_VIOLENCE': ['498A', '304B', '406']
}

STATIONS = [
    ('STATION-001', 'Connaught Place', 'New Delhi'),
    ('STATION-002', 'Cyber Crime Cell', 'Delhi'),
    ('STATION-003', 'Koramangala', 'Bangalore'),
    ('STATION-004', 'Colaba', 'Mumbai'),
    ('STATION-005', 'Park Street', 'Kolkata'),
]

PRIORITIES = ['LOW', 'MEDIUM', 'HIGH', 'CRITICAL']
FIR_STATUSES = ['REGISTERED', 'UNDER_INVESTIGATION', 'PENDING', 'CLOSED']
CASE_STATUSES = ['OPEN', 'UNDER_INVESTIGATION', 'TRIAL', 'CLOSED']

def get_db_connection():
    """Create database connection"""
    # Parse DATABASE_URL
    import re
    match = re.match(r'postgres://([^:]+):([^@]+)@([^:]+):(\d+)/([^?]+)', DB_URL)
    if match:
        user, password, host, port, dbname = match.groups()
        return psycopg2.connect(
            host=host,
            port=port,
            dbname=dbname,
            user=user,
            password=password
        )
    raise ValueError("Invalid DATABASE_URL format")

def seed_firs(conn, count=50):
    """Generate FIR records"""
    print(f"Generating {count} FIRs...")

    firs = []
    for i in range(count):
        station_id, station_name, district = random.choice(STATIONS)
        category = random.choice(CRIME_CATEGORIES)
        incident_date = fake.date_time_between(start_date='-6M', end_date='now')
        registered_at = incident_date + timedelta(hours=random.randint(1, 48))

        fir = (
            f"FIR/{station_id.split('-')[1]}/{datetime.now().year}/{i+1:04d}",  # fir_number
            category,
            random.choice(IPC_SECTIONS[category]),  # ipc_sections
            fake.sentence(nb_words=10),  # title
            fake.paragraph(nb_sentences=5),  # description
            incident_date,
            fake.address(),  # incident_location
            json.dumps({'lat': fake.latitude(), 'lng': fake.longitude()}),  # coordinates
            fake.name(),  # complainant_name
            fake.phone_number(),  # complainant_contact
            fake.address(),  # complainant_address
            random.choice(PRIORITIES),
            random.choice(FIR_STATUSES),
            station_id,
            district,
            registered_at,
            None,  # investigating_officer_id (will set later)
            None,  # case_id (will set later)
            fake.paragraph(nb_sentences=2),  # remarks
        )
        firs.append(fir)

    with conn.cursor() as cur:
        execute_values(
            cur,
            """
            INSERT INTO firs (
                fir_number, crime_category, ipc_sections, title, description,
                incident_date, incident_location, coordinates, complainant_name,
                complainant_contact, complainant_address, priority, status,
                station_id, district, registered_at, investigating_officer_id,
                case_id, remarks
            ) VALUES %s
            RETURNING id
            """,
            firs
        )
        fir_ids = [row[0] for row in cur.fetchall()]

    conn.commit()
    print(f"✓ Created {len(fir_ids)} FIRs")
    return fir_ids

def seed_cases(conn, fir_ids, count=30):
    """Generate case records"""
    print(f"Generating {count} cases...")

    cases = []
    used_fir_ids = random.sample(fir_ids, min(count, len(fir_ids)))

    for i, fir_id in enumerate(used_fir_ids):
        station_id = random.choice(STATIONS)[0]
        registered_date = fake.date_time_between(start_date='-5M', end_date='now')

        case = (
            f"CASE/{station_id.split('-')[1]}/{datetime.now().year}/{i+1:04d}",  # case_number
            fir_id,
            random.choice(CASE_STATUSES),
            fake.sentence(nb_words=8),  # title
            fake.paragraph(nb_sentences=4),  # description
            registered_date,
            station_id,
            None,  # investigating_officer_id
            None,  # court_name (will set later)
            None,  # next_hearing_date
            fake.paragraph(nb_sentences=2),  # remarks
        )
        cases.append(case)

    with conn.cursor() as cur:
        execute_values(
            cur,
            """
            INSERT INTO cases (
                case_number, fir_id, status, title, description,
                registered_date, station_id, investigating_officer_id,
                court_name, next_hearing_date, remarks
            ) VALUES %s
            RETURNING id
            """,
            cases
        )
        case_ids = [row[0] for row in cur.fetchall()]

    conn.commit()
    print(f"✓ Created {len(case_ids)} cases")
    return case_ids

def seed_evidence(conn, case_ids, count=80):
    """Generate evidence records"""
    print(f"Generating {count} evidence items...")

    evidence_types = ['PHYSICAL', 'DIGITAL', 'DOCUMENT', 'PHOTO', 'VIDEO']
    evidence_items = []

    for i in range(count):
        case_id = random.choice(case_ids)
        evidence_type = random.choice(evidence_types)
        collected_date = fake.date_time_between(start_date='-4M', end_date='now')

        evidence = (
            f"EVD/{datetime.now().year}/{i+1:05d}",  # evidence_number
            case_id,
            evidence_type,
            fake.sentence(nb_words=6),  # description
            fake.address(),  # collection_location
            collected_date,
            fake.name(),  # collected_by
            json.dumps({'hash': fake.sha256(), 'size': random.randint(1000, 10000000)}),  # storage_location
            random.choice(['SECURE', 'PENDING', 'RELEASED']),  # chain_of_custody_status
            fake.paragraph(nb_sentences=2),  # remarks
        )
        evidence_items.append(evidence)

    with conn.cursor() as cur:
        execute_values(
            cur,
            """
            INSERT INTO evidence (
                evidence_number, case_id, type, description, collection_location,
                collected_date, collected_by, storage_location, chain_of_custody_status, remarks
            ) VALUES %s
            RETURNING id
            """,
            evidence_items
        )
        evidence_ids = [row[0] for row in cur.fetchall()]

    conn.commit()
    print(f"✓ Created {len(evidence_ids)} evidence items")
    return evidence_ids

def seed_warrants(conn, case_ids, count=25):
    """Generate warrant records"""
    print(f"Generating {count} warrants...")

    warrant_types = ['ARREST', 'SEARCH', 'SUMMONS', 'NBW']
    warrant_statuses = ['ACTIVE', 'EXECUTED', 'EXPIRED', 'CANCELLED']

    warrants = []
    for i in range(count):
        case_id = random.choice(case_ids)
        warrant_type = random.choice(warrant_types)
        issue_date = fake.date_time_between(start_date='-3M', end_date='now')
        expiry_date = issue_date + timedelta(days=random.randint(30, 180))

        warrant = (
            f"WRT/{datetime.now().year}/{i+1:05d}",  # warrant_number
            warrant_type,
            random.choice(warrant_statuses),
            case_id,
            fake.name(),  # subject_name
            fake.address(),  # subject_address
            issue_date,
            expiry_date,
            fake.name(),  # issued_by (judge name)
            fake.sentence(nb_words=8),  # purpose
            None,  # executed_date
            None,  # executed_by
            fake.paragraph(nb_sentences=2),  # remarks
        )
        warrants.append(warrant)

    with conn.cursor() as cur:
        execute_values(
            cur,
            """
            INSERT INTO warrants (
                warrant_number, type, status, case_id, subject_name, subject_address,
                issue_date, expiry_date, issued_by, purpose, executed_date, executed_by, remarks
            ) VALUES %s
            RETURNING id
            """,
            warrants
        )
        warrant_ids = [row[0] for row in cur.fetchall()]

    conn.commit()
    print(f"✓ Created {len(warrant_ids)} warrants")
    return warrant_ids

def seed_alerts(conn, count=15):
    """Generate alert records"""
    print(f"Generating {count} alerts...")

    alert_types = ['SECURITY', 'LOOKOUT', 'URGENT', 'MISSING_PERSON', 'WANTED']
    alert_scopes = ['NATIONAL', 'STATE', 'DISTRICT', 'STATION']

    alerts = []
    for i in range(count):
        alert_type = random.choice(alert_types)
        created_at = fake.date_time_between(start_date='-2M', end_date='now')
        expires_at = created_at + timedelta(days=random.randint(7, 90))

        alert = (
            f"ALERT/{datetime.now().year}/{i+1:04d}",  # alert_number
            alert_type,
            random.choice(alert_scopes),
            fake.sentence(nb_words=8),  # title
            fake.paragraph(nb_sentences=3),  # message
            random.choice(PRIORITIES),
            random.choice([True, False]),  # active
            random.choice([True, False]),  # acknowledged
            created_at,
            expires_at,
            fake.name(),  # created_by
        )
        alerts.append(alert)

    with conn.cursor() as cur:
        execute_values(
            cur,
            """
            INSERT INTO alerts (
                alert_number, type, scope, title, message, priority,
                active, acknowledged, created_at, expires_at, created_by
            ) VALUES %s
            RETURNING id
            """,
            alerts
        )
        alert_ids = [row[0] for row in cur.fetchall()]

    conn.commit()
    print(f"✓ Created {len(alert_ids)} alerts")
    return alert_ids

def seed_personnel(conn, count=20):
    """Generate personnel records"""
    print(f"Generating {count} personnel...")

    ranks = ['CONSTABLE', 'HEAD_CONSTABLE', 'SI', 'INSPECTOR', 'DSP', 'SP']
    departments = ['CRIME', 'TRAFFIC', 'CYBER', 'SPECIAL_BRANCH', 'ADMINISTRATION']

    personnel = []
    for i in range(count):
        join_date = fake.date_time_between(start_date='-15y', end_date='-1y')

        person = (
            f"EMP{datetime.now().year}{i+1:04d}",  # employee_id
            fake.name(),  # name
            random.choice(ranks),
            random.choice(departments),
            random.choice(STATIONS)[0],  # station_id
            fake.phone_number(),  # contact
            fake.email(),  # email
            join_date,
            random.choice(['ACTIVE', 'ON_LEAVE', 'SUSPENDED']),
            fake.address(),  # address
        )
        personnel.append(person)

    with conn.cursor() as cur:
        execute_values(
            cur,
            """
            INSERT INTO personnel (
                employee_id, name, rank, department, station_id,
                contact, email, join_date, status, address
            ) VALUES %s
            RETURNING id
            """,
            personnel
        )
        personnel_ids = [row[0] for row in cur.fetchall()]

    conn.commit()
    print(f"✓ Created {len(personnel_ids)} personnel")
    return personnel_ids

def main():
    """Main seed function"""
    print("=" * 70)
    print("NPDMS DEMO DATA SEED SCRIPT")
    print("=" * 70)
    print()

    try:
        conn = get_db_connection()
        print("✓ Connected to database")
        print()

        # Seed in order of dependencies
        fir_ids = seed_firs(conn, count=50)
        case_ids = seed_cases(conn, fir_ids, count=30)
        evidence_ids = seed_evidence(conn, case_ids, count=80)
        warrant_ids = seed_warrants(conn, case_ids, count=25)
        alert_ids = seed_alerts(conn, count=15)
        personnel_ids = seed_personnel(conn, count=20)

        print()
        print("=" * 70)
        print("DEMO DATA SEED COMPLETE!")
        print("=" * 70)
        print(f"Total FIRs:      {len(fir_ids)}")
        print(f"Total Cases:     {len(case_ids)}")
        print(f"Total Evidence:  {len(evidence_ids)}")
        print(f"Total Warrants:  {len(warrant_ids)}")
        print(f"Total Alerts:    {len(alert_ids)}")
        print(f"Total Personnel: {len(personnel_ids)}")
        print()
        print("You can now login and explore the demo data!")
        print()

        conn.close()

    except Exception as e:
        print(f"✗ Error: {e}")
        import traceback
        traceback.print_exc()
        return 1

    return 0

if __name__ == '__main__':
    exit(main())
