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


# Transfer process state transitions

## Transitions


| From      | To         | Transition URL                                                |  Actor   | Comment |
| --------- | ---------- | ------------------------------------------------------------- | -------- | ---------------------------------------- |
| X         | REQUESTED  | provider.dsp/transfers/request                                | Consumer |                                          |
| REQUESTED | STARTED    | consumer.dsp/:callback/transfers/:consumerPID/start           | Provider |                                          |
| STARTED   | COMPLETED  | consumer.dsp/transfers/:providerPID/completion                | Consumer | Used in pull mode?                       |
| STARTED   | COMPLETED  | consumer.dsp/:callback/transfers/:consumerPID/completion      | Consumer | Used in push mode?                       |
| STARTED   | SUSPENDED  | consumer.dsp/:callback/transfers/:consumerPID/suspension      | Provider |                                          |
| SUSPENDED | STARTED    | consumer.dsp/:callback/transfers/:consumerPID/start           | Provider |                                          |
| STARTED   | SUSPENDED  | provider.dsp/transfers/:providerPID/suspension                | Consumer |                                          |
| SUSPENDED | STARTED    | provider.dsp/transfers/:providerPID/start                     | Consumer |                                          |
| *         | TERMINATED | consumer.dsp/negotiatons/:consumerPID/termination             | Provider | No callback prefix?                      |
| *         | TERMINATED | provider.dsp/negotiatons/:providerPID/termination             | Consumer |                                          |

## Pull flow

| From      | To         | Transition URL                                                |  Actor   | Comment |
| --------- | ---------- | ------------------------------------------------------------- | -------- | ---------------------------------------- |
| X         | REQUESTED  | provider.dsp/transfers/request                                | Consumer |                                          |
| REQUESTED | STARTED    | consumer.dsp/:callback/transfers/:consumerPID/start           | Provider |                                          |
| STARTED   | COMPLETED  | consumer.dsp/transfers/:providerPID/completion                | Consumer | Used in pull mode?                       |

## Push flow

| From      | To         | Transition URL                                                |  Actor   | Comment |
| --------- | ---------- | ------------------------------------------------------------- | -------- | ---------------------------------------- |
| X         | REQUESTED  | provider.dsp/transfers/request                                | Consumer |                                          |
| REQUESTED | STARTED    | consumer.dsp/:callback/transfers/:consumerPID/start           | Provider |                                          |
| STARTED   | COMPLETED  | consumer.dsp/:callback/transfers/:consumerPID/completion      | Consumer | Used in push mode?                       |
