# eSSL Biometric Attendance System - Architecture & Database Findings

## Overview

This document summarizes how a typical **eSSL biometric attendance system** works, where the attendance data is stored, how the biometric device communicates with the server, and how to identify the SQL Server database and attendance tables.

---

# High-Level Architecture

```text
+-------------------------+
| eSSL Biometric Device   |
| (Fingerprint/Face)      |
+------------+------------+
             |
             | TCP/IP (Wi-Fi/LAN)
             | Port: 4370
             |
             v
+-------------------------+
| eTimeTrack/eTimeTrack   |
| Communication Service   |
+------------+------------+
             |
             | Inserts attendance logs
             |
             v
+-------------------------+
| Microsoft SQL Server    |
| Attendance Database     |
+------------+------------+
             |
             |
             v
+-------------------------+
| eTimeTrack Application  |
| Reports / HR Portal     |
+-------------------------+
```

---

# Data Flow

1. Employee punches on the biometric device.
2. Device stores the punch in its internal memory.
3. eTimeTrack Communication Service periodically connects to the device over TCP/IP.
4. New punch logs are downloaded.
5. Logs are inserted into SQL Server.
6. HR software generates attendance reports from SQL Server.

> **Important:** The biometric device does **not** directly write to SQL Server. The eTimeTrack software acts as the intermediary.

---

# Where Is the Database Located?

Most commonly:

- On the HR Windows PC
- On a dedicated Windows Server (large organizations)

Typical databases:

- SQL Server Express
- Microsoft SQL Server Standard

Older installations may use:

- MySQL
- Microsoft Access (.mdb)

---

# Communication Details

Protocol:

```
TCP/IP
```

Default Port:

```
4370
```

Connection Types:

- Wi-Fi
- Ethernet (LAN)

---

# Device Storage

The biometric device stores:

- User IDs
- Fingerprint templates
- Face templates (if supported)
- Recent attendance logs

The device **does not** maintain long-term attendance history.

The SQL Server database becomes the system of record after synchronization.

---

# Typical Software Components

```
eSSL Device
        ↓
eTimeTrack Communication Service
        ↓
SQL Server
        ↓
eTimeTrack Desktop/Web Application
```

---

# Finding the SQL Server Instance

## Method 1

Open:

```
services.msc
```

Look for services such as:

```
SQL Server (SQLEXPRESS)
SQL Server (MSSQLSERVER)
SQL Server (ESSL)
SQL Server (ETIMETRACK)
```

Example:

```
SQL Server (SQLEXPRESS)
```

Instance name:

```
localhost\SQLEXPRESS
```

---

## Method 2

Open SQL Server Configuration Manager.

Look under:

```
SQL Server Services
```

---

# Finding the Database Connection

Check configuration files inside:

```
C:\Program Files\eSSL\
```

or

```
C:\Program Files (x86)\eSSL\
```

Look for files like:

- config.ini
- database.ini
- settings.xml
- app.config
- web.config

Search for:

```
Server=
Database=
Data Source=
Initial Catalog=
```

Example:

```ini
Server=localhost\SQLEXPRESS
Database=eTimeTrackLite
User ID=sa
Password=******
```

---

# Connecting Using SQL Server Management Studio (SSMS)

Common server names:

```
localhost
```

or

```
.\SQLEXPRESS
```

or

```
localhost\SQLEXPRESS
```

Authentication:

- Windows Authentication
- SQL Authentication

---

# Listing Databases

```sql
SELECT name
FROM sys.databases;
```

Possible database names:

```
eTimeTrackLite
eTimeTrack
ESSL
ZKEM
Attendance
```

---

# Listing Tables

```sql
USE eTimeTrackLite;
GO

SELECT name
FROM sys.tables
ORDER BY name;
```

---

# Common eSSL Tables

| Table | Purpose |
|--------|---------|
| USERINFO | Employee information |
| CHECKINOUT | Attendance punch logs |
| Machines | Registered biometric devices |
| Departments | Departments |
| Shift / NUM_RUN | Shift configuration |
| LeaveClass | Leave types |
| Holiday | Holidays |
| CHECKEXACT | Attendance processing |

Actual names may vary slightly depending on software version.

---

# Attendance Log Table

Most commonly:

```
CHECKINOUT
```

Example query:

```sql
SELECT TOP 100 *
FROM CHECKINOUT
ORDER BY CHECKTIME DESC;
```

Typical columns:

| Column | Description |
|---------|-------------|
| USERID | Employee ID |
| CHECKTIME | Punch timestamp |
| CHECKTYPE | IN / OUT |
| VERIFYCODE | Verification method |
| SENSORID | Device ID |

---

# Employee Table

Usually:

```
USERINFO
```

Example:

```sql
SELECT TOP 100 *
FROM USERINFO;
```

Typical columns:

| Column | Description |
|---------|-------------|
| USERID | Internal employee ID |
| BADGENUMBER | Employee code |
| NAME | Employee name |
| DEFAULTDEPTID | Department |
| SSN | Employee identifier (optional) |

---

# Joining Attendance with Employee Details

```sql
SELECT
    u.BADGENUMBER,
    u.NAME,
    c.CHECKTIME,
    c.CHECKTYPE
FROM CHECKINOUT c
JOIN USERINFO u
ON c.USERID = u.USERID
ORDER BY c.CHECKTIME DESC;
```

---

# Registered Devices

Usually stored in:

```
Machines
```

Example:

```sql
SELECT *
FROM Machines;
```

Possible fields:

| MachineAlias | IP | Port |
|--------------|----|------|
| Main Gate | 192.168.1.120 | 4370 |

---

# Finding Tables When Names Are Unknown

```sql
SELECT
    t.name AS TableName,
    c.name AS ColumnName
FROM sys.tables t
JOIN sys.columns c
ON t.object_id = c.object_id
WHERE c.name IN
(
'CHECKTIME',
'USERID',
'CHECKTYPE',
'BADGENUMBER'
);
```

---

# Recommended Cloud Synchronization Architecture

Instead of reading directly from the biometric device, synchronize from SQL Server.

```
eSSL Device
        │
        ▼
eTimeTrack Communication Service
        │
        ▼
SQL Server
        │
        ▼
Go Sync Service
        │
        ▼
Kafka / REST API
        │
        ▼
Cloud Database
```

---

# Why Sync from SQL Server Instead of the Device?

Advantages:

- No direct communication with biometric devices.
- Avoids interfering with HR software.
- SQL Server already contains cleaned and synchronized attendance data.
- Easier incremental synchronization.
- Better scalability.
- Centralized integration point.

---

# Incremental Sync Strategy

Recommended:

- Track the latest processed attendance record.
- Fetch only newly inserted records.

Example:

```sql
SELECT *
FROM CHECKINOUT
WHERE CHECKTIME > @LastSyncTime
ORDER BY CHECKTIME;
```

If the table has an identity column:

```sql
SELECT *
FROM CHECKINOUT
WHERE LogID > @LastLogID
ORDER BY LogID;
```

---

# Key Findings

- eSSL devices communicate over TCP/IP (default port 4370).
- Devices store fingerprints, users, and temporary attendance logs.
- eTimeTrack software downloads attendance logs from devices.
- SQL Server is the primary long-term storage.
- `CHECKINOUT` is typically the attendance log table.
- `USERINFO` stores employee details.
- `Machines` stores configured biometric devices.
- Cloud integrations should read from SQL Server rather than directly from biometric devices for reliability and scalability.