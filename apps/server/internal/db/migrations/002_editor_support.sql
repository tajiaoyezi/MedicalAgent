-- c02 editor support: conversion cache only
-- document_parse_jobs owner=c03（tasks 1.5），c02 不在此建表
-- 历史 stub 由 003 清理；全仓无 writer，stub 恒空，DROP 安全

CREATE TABLE IF NOT EXISTS editor_conversion_cache (
  source_hash TEXT PRIMARY KEY,
  target_object_key TEXT NOT NULL,
  target_mime TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
