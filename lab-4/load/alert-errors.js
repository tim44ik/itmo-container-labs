import http from 'k6/http';

export const options = {
  scenarios: {
    errors: {
      executor: 'constant-arrival-rate',
      rate: 2,
      timeUnit: '1s',
      duration: '5m',
      preAllocatedVUs: 2,
      maxVUs: 5,
    },
  },
};

// Для этого эксперимента 500 — намеренно ожидаемый ответ.
// Prometheus всё равно учитывает его как HTTP 5xx.
http.setResponseCallback(http.expectedStatuses(500));

export default function () {
  http.get(`${__ENV.BASE_URL}/fail`, { timeout: '5s' });
}
