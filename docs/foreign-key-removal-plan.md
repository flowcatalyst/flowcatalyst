# Removing the foreign keys (superseded)

This plan (2026-08-27: remove every foreign key) is superseded by the owner ruling of 2026-11-10:
foreign keys stay between master and configuration tables, and transactional tables carry none.

The policy, the audit of every foreign key, the keep/drop decision for each, and the rule for new
tables are in [`foreign-keys.md`](foreign-keys.md). Migration 114 dropped the transactional ones.
