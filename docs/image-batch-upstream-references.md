# Image Batch Upstream References

This document records the exact upstream revisions consulted for the AI creation center image batching work. These projects are references only; none is added as a source or runtime dependency.

| Project | Version | Verified commit | License | Reference scope |
| --- | --- | --- | --- | --- |
| [OpenAI Python](https://github.com/openai/openai-python) | `v3.8.0` | `88391abf981df3ea395ca1b5bf55ec6a4011ea93` | Apache-2.0 | Images edit multipart construction, repeated image inputs, `n`, and `input_fidelity` request fields. |
| [LiteLLM](https://github.com/BerriAI/litellm) | `v1.99.1` | `10f4033437df30b91b5dbf2b64711d0a8683fc52` | MIT outside `enterprise/` | Provider batch-count forwarding and normalization of multi-image response lists. No `enterprise/` code was consulted. |
| [Google Gen AI Python](https://github.com/googleapis/python-genai) | `v2.22.0` | `0ec3d8a4b2c85817434045dad739f6227c2d5c4c` | Apache-2.0 | Ordered `inline_data` parts, `candidate_count`, candidates, and image response-part traversal. |

Implementation remains native Go and TypeScript in this repository. The referenced SDKs were not vendored, copied, installed, or linked into the application.
