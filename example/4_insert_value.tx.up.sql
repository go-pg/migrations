SET statement_timeout = 60000; -- 60 seconds
SET lock_timeout = 60000; -- 60 seconds

--gopg:split

INSERT INTO my_table VALUES (2);
