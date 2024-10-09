-- SELECT
-- 	pair,
-- 	side,
-- 	SUM( profit_value ) AS profit_sum
-- FROM
-- 	positions
-- GROUP BY
-- 	pair,
-- 	side
-- ORDER BY
-- 	pair,side,profit_sum DESC;

SELECT
    p.pair,
    p.side,
    p.profit_value,
    p.updated_at,
    ps.open_params
FROM
    positions AS p
        LEFT JOIN position_strategies AS ps ON p.order_flag = ps.order_flag
WHERE
    p.pair IN (
        SELECT pair
        FROM (
                 SELECT
                     pair,
                     SUM(profit_value) AS profit_sum
                 FROM
                     positions
                 GROUP BY
                     pair
                 HAVING
                     profit_sum < 0
             ) AS b
    )
ORDER BY
    p.pair ASC,
    p.side,
    p.profit_value DESC,
    p.updated_at ASC;