<!--
 Copyright 2024 go-dataspace
 
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
 
     https://www.apache.org/licenses/LICENSE-2.0
 
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
-->

# Contract negotiation state transitions

## Transitions

| From      | To         | Transition URL                                                |  Actor   | Comment |
| --------- | ---------- | ------------------------------------------------------------- | -------- | ---------------------------------------- |
| X         | REQUESTED  | provider.dsp/negotiations/request                             | Consumer |                                          |
| X         | OFFERED    | consumer.dsp/negotiations/offers                              | Provider | This includes the consumer callback url? |
| REQUESTED | OFFERED    | consumer.dsp/:callback/negotiations/:consumerPID/offers       | Provider |                                          |
| REQUESTED | AGREED     | consumer.dsp/:callback/negotiations/:consumerPID/agreement    | Provider |                                          |
| OFFERED   | REQUESTED  | provider.dsp/negotiations/:providerPID/request                | Consumer |                                          |
| OFFERED   | ACCEPTED   | provider.dsp/negotiations/:providerPID/events                 | Consumer |                                          |
| ACCEPTED  | AGREED     | consumer.dsp/:callback/negotiations/:consumerPID/agreement    | Provider |                                          |
| AGREED    | VERIFIED   | provider.dsp/negotiations/:providerPID/agreement/verification | Consumer |                                          |
| VERIFIED  | FINALIZED  | consumer.dsp/:callback/:consumerPID/events                    | Provider |                                          |
| *         | TERMINATED | consumer.dsp/negotiatons/:consumerPID/termination             | Provider | No callback prefix?                      |
| *         | TERMINATED | provider.dsp/negotiatons/:providerPID/termination             | Consumer |                                          |

## Consumer initiated path

| From      | To         | Transition URL                                                |  Actor   | Comment |
| --------- | ---------- | ------------------------------------------------------------- | -------- | ---------------------------------------- |
| X         | REQUESTED  | provider.dsp/negotiations/request                             | Consumer |                                          |
| REQUESTED | OFFERED    | consumer.dsp/:callback/negotiations/:consumerPID/offers       | Provider |                                          |
| OFFERED   | ACCEPTED   | provider.dsp/negotiations/:providerPID/events                 | Consumer |                                          |
| ACCEPTED  | AGREED     | consumer.dsp/:callback/negotiations/:consumerPID/agreement    | Provider |                                          |
| AGREED    | VERIFIED   | provider.dsp/negotiations/:providerPID/agreement/verification | Consumer |                                          |
| VERIFIED  | FINALIZED  | consumer.dsp/:callback/:consumerPID/events                    | Provider |                                          |

### Shortened

Unclear if the jump from REQUESTED to AGREED is made for this, just for the provider initiated
path, or both.

| From      | To         | Transition URL                                                |  Actor   | Comment |
| --------- | ---------- | ------------------------------------------------------------- | -------- | ---------------------------------------- |
| X         | REQUESTED  | provider.dsp/negotiations/request                             | Consumer |                                          |
| REQUESTED | AGREED     | consumer.dsp/:callback/negotiations/:consumerPID/agreement    | Provider |                                          |
| AGREED    | VERIFIED   | provider.dsp/negotiations/:providerPID/agreement/verification | Consumer |                                          |
| VERIFIED  | FINALIZED  | consumer.dsp/:callback/:consumerPID/events                    | Provider |                                          |

## Provider initiated path

This has a weird retread on offered, so I think the shortened version is made for this.

| From      | To         | Transition URL                                                |  Actor   | Comment |
| --------- | ---------- | ------------------------------------------------------------- | -------- | ---------------------------------------- |
| X         | OFFERED    | consumer.dsp/negotiations/offers                              | Provider | This includes the consumer callback url? |
| OFFERED   | REQUESTED  | provider.dsp/negotiations/:providerPID/request                | Consumer |                                          |
| REQUESTED | OFFERED    | consumer.dsp/:callback/negotiations/:consumerPID/offers       | Provider |                                          |
| OFFERED   | ACCEPTED   | provider.dsp/negotiations/:providerPID/events                 | Consumer |                                          |
| ACCEPTED  | AGREED     | consumer.dsp/:callback/negotiations/:consumerPID/agreement    | Provider |                                          |
| AGREED    | VERIFIED   | provider.dsp/negotiations/:providerPID/agreement/verification | Consumer |                                          |
| VERIFIED  | FINALIZED  | consumer.dsp/:callback/:consumerPID/events                    | Provider |                                          |

### Shortened

This seems like the likely path as it doesn't include that weird loop.

| From      | To         | Transition URL                                                |  Actor   | Comment |
| --------- | ---------- | ------------------------------------------------------------- | -------- | ---------------------------------------- |
| X         | OFFERED    | consumer.dsp/negotiations/offers                              | Provider | This includes the consumer callback url? |
| OFFERED   | REQUESTED  | provider.dsp/negotiations/:providerPID/request                | Consumer |                                          |
| REQUESTED | AGREED     | consumer.dsp/:callback/negotiations/:consumerPID/agreement    | Provider |                                          |
| AGREED    | VERIFIED   | provider.dsp/negotiations/:providerPID/agreement/verification | Consumer |                                          |
| VERIFIED  | FINALIZED  | consumer.dsp/:callback/:consumerPID/events                    | Provider |                                          |


