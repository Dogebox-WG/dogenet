module github.com/dogeorg/dogenet

require rad/governor v1.0.0
require rad/gossip v1.0.0

replace rad/governor => ash.radicle.garden/z3iGsD9SQumb7dHQgwmfLQ6JTAK4X.git v0.0.0-20240716041207-e497dff071a6
//replace rad/gossip => ash.radicle.garden/z4H8iGjfSyWNpc4F1YHGhjY3h335J.git   v0.0.0-20240718005822-4c979a430c9e

replace rad/gossip => ../gossip

go 1.22.5
