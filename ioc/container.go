/*
Package ioc provides an Inversion of Control component container and lifecycle hooks.
*/
package ioc

import (
	"errors"
	"fmt"
	"github.com/graniticio/granitic/config"
	"github.com/graniticio/granitic/instance"
	"github.com/graniticio/granitic/logging"
	"github.com/graniticio/granitic/reflecttools"
	"os"
	"time"
)

const containerDecoratorComponentName = instance.FrameworkPrefix + "ContainerDecorator"
const containerComponentName = instance.FrameworkPrefix + "Container"

type ComponentByNameFinder interface {
	ComponentByName(string) *Component
}

func NewComponentContainer(loggingManager *logging.ComponentLoggerManager, configAccessor *config.ConfigAccessor) *ComponentContainer {

	cc := new(ComponentContainer)
	cc.protoComponents = make(map[string]*ProtoComponent)
	cc.FrameworkLogger = loggingManager.CreateLogger(containerComponentName)
	cc.configAccessor = configAccessor
	cc.modifiers = make(map[string]map[string]string)

	return cc

}

type ComponentContainer struct {
	allComponents   map[string]*Component
	protoComponents map[string]*ProtoComponent
	FrameworkLogger logging.Logger
	configAccessor  *config.ConfigAccessor
	startable       []*Component
	stoppable       []*Component
	blocker         []*Component
	accessible      []*Component
	modifiers       map[string]map[string]string
}

func (cc *ComponentContainer) ComponentByName(name string) *Component {
	return cc.allComponents[name]
}

func (cc *ComponentContainer) AllComponents() map[string]*Component {
	return cc.allComponents
}

func (cc *ComponentContainer) AddModifier(comp string, field string, dep string) {

	m := cc.modifiers
	cm := m[comp]

	if cm == nil {
		cm = make(map[string]string)
		m[comp] = cm
	}

	cm[field] = dep

}

func (cc *ComponentContainer) AddModifiers(mods map[string]map[string]string) {

	for c, cm := range mods {

		for f, d := range cm {
			cc.AddModifier(c, f, d)
		}

	}

}

func (cc *ComponentContainer) ModifierExists(comp string, field string) bool {

	m := cc.modifiers[comp]

	return m != nil && m[field] != ""

}

func (cc *ComponentContainer) ModifiersExist(comp string) bool {
	return cc.modifiers[comp] != nil
}

func (cc *ComponentContainer) Modifiers(comp string) map[string]string {
	return cc.modifiers[comp]
}

func (cc *ComponentContainer) AddProto(proto *ProtoComponent) {

	cc.FrameworkLogger.LogTracef("Adding proto %s", proto.Component.Name)

	cc.protoComponents[proto.Component.Name] = proto
}

func (cc *ComponentContainer) WrapAndAddProto(name string, instance interface{}) {
	p := CreateProtoComponent(instance, name)
	cc.AddProto(p)
}

func (cc *ComponentContainer) AddProtos(protos []*ProtoComponent) {
	for _, p := range protos {
		cc.AddProto(p)
	}
}

func (cc *ComponentContainer) StartComponents() error {

	defer func() {
		if r := recover(); r != nil {
			cc.FrameworkLogger.LogErrorfWithTrace("Panic recovered while starting components components %s", r)
			os.Exit(-1)
		}
	}()

	for _, component := range cc.startable {

		startable := component.Instance.(Startable)

		err := startable.StartComponent()

		if err != nil {
			message := fmt.Sprintf("Unable to start %s: %s", component.Name, err)
			return errors.New(message)
		}

	}

	if len(cc.blocker) != 0 {
		err := cc.waitForBlockers(5*time.Second, 12, 0)

		if err != nil {
			return err
		}

	}

	for _, component := range cc.accessible {

		accessible := component.Instance.(Accessible)
		err := accessible.AllowAccess()

		if err != nil {
			return err
		}

	}

	cc.startable = nil
	cc.configAccessor = nil

	return nil
}

func (cc *ComponentContainer) waitForBlockers(retestInterval time.Duration, maxTries int, warnAfterTries int) error {

	var names []string

	for i := 0; i < maxTries; i++ {

		notReady, cNames := cc.countBlocking(i > warnAfterTries)
		names = cNames

		if notReady == 0 {
			return nil
		} else {
			time.Sleep(retestInterval)
		}
	}

	message := fmt.Sprintf("Startup blocked by %v", names)

	return errors.New(message)

}

func (cc *ComponentContainer) ShutdownComponents() error {

	for _, c := range cc.stoppable {

		s := c.Instance.(Stoppable)
		s.PrepareToStop()
	}

	cc.waitForReadyToStop(5*time.Second, 10, 3)

	for _, c := range cc.stoppable {

		s := c.Instance.(Stoppable)
		err := s.Stop()

		if err != nil {
			cc.FrameworkLogger.LogErrorf("%s did not stop cleanly %s", c.Name, err)
		}

	}

	return nil

}

func (cc *ComponentContainer) waitForReadyToStop(retestInterval time.Duration, maxTries int, warnAfterTries int) {

	for i := 0; i < maxTries; i++ {

		notReady := cc.countNotReady(i > warnAfterTries)

		if notReady == 0 {
			return
		} else {
			time.Sleep(retestInterval)
		}
	}

	cc.FrameworkLogger.LogFatalf("Some components not ready to stop, stopping anyway")

}

func (cc *ComponentContainer) countBlocking(warn bool) (int, []string) {

	notReady := 0
	names := []string{}

	for _, c := range cc.blocker {
		ab := c.Instance.(AccessibilityBlocker)

		block, err := ab.BlockAccess()

		if block {
			notReady += 1
			names = append(names, c.Name)
			if warn {
				if err != nil {
					cc.FrameworkLogger.LogErrorf("%s blocking startup: %s", c.Name, err)
				} else {
					cc.FrameworkLogger.LogErrorf("%s blocking startup (no reason given)", c.Name)
				}

			}
		}

	}

	return notReady, names
}

func (cc *ComponentContainer) countNotReady(warn bool) int {

	notReady := 0

	for _, c := range cc.stoppable {
		s := c.Instance.(Stoppable)

		ready, err := s.ReadyToStop()

		if !ready {
			notReady += 1

			if warn {
				if err != nil {
					cc.FrameworkLogger.LogWarnf("%s is not ready to stop: %s", c.Name, err)
				} else {
					cc.FrameworkLogger.LogWarnf("%s is not ready to stop (no reason given)", c.Name)
				}

			}
		}

	}

	return notReady
}

func (cc *ComponentContainer) Populate() error {

	defer func() {
		if r := recover(); r != nil {
			cc.FrameworkLogger.LogErrorfWithTrace("Panic recovered while configuring components %s", r)
			os.Exit(-1)
		}
	}()

	decorators := make(map[string]ComponentDecorator)

	containerDecorator := new(ContainerDecorator)
	containerDecorator.container = cc

	decorators[containerDecoratorComponentName] = containerDecorator

	cc.allComponents = make(map[string]*Component)

	for _, protoComponent := range cc.protoComponents {

		component := protoComponent.Component

		if !reflecttools.IsPointerToStruct(component.Instance) {
			m := fmt.Sprintf("Component %s is not a pointer to a struct.", component.Name)
			return errors.New(m)
		}

		cc.addComponent(component)
		cc.captureDecorator(component, decorators)
	}

	err := cc.resolveDependenciesAndConfig()

	if err != nil {
		cc.FrameworkLogger.LogFatalf(err.Error())
		cc.FrameworkLogger.LogInfof("Aborting startup")
		os.Exit(-1)
	}

	cc.decorateComponents(decorators)

	cc.protoComponents = nil

	return nil
}

func (cc *ComponentContainer) resolveDependenciesAndConfig() error {

	fl := cc.FrameworkLogger

	for _, targetProto := range cc.protoComponents {

		compName := targetProto.Component.Name
		deps := cc.mergeDependencies(compName, targetProto.Dependencies)

		for fieldName, depName := range deps {

			fl.LogTracef("%s needs %s", compName, depName)

			requiredComponent := cc.allComponents[depName]

			if requiredComponent == nil {
				message := fmt.Sprintf("No component named %s available (required by %s.%s)", depName, compName, fieldName)
				return errors.New(message)
			}

			targetInstance := targetProto.Component.Instance
			requiredInstance := requiredComponent.Instance

			err := reflecttools.SetPtrToStruct(targetInstance, fieldName, requiredInstance)

			if err != nil {
				m := fmt.Sprintf("Problem injecting dependency '%s' into %s.%s: %s", depName, compName, fieldName, err.Error())
				return errors.New(m)
			}

		}

		for fieldName, configPath := range targetProto.ConfigPromises {
			fl.LogTracef("%s needs %s", targetProto.Component.Name, fieldName, configPath)

			cc.configAccessor.SetField(fieldName, configPath, targetProto.Component.Instance)

		}

	}

	return nil
}

// Combines dependencies attached to the proto components with any available framework modifiers
func (cc *ComponentContainer) mergeDependencies(comp string, cd map[string]string) map[string]string {

	merged := make(map[string]string)

	for k, v := range cd {
		merged[k] = v
	}

	if cc.ModifiersExist(comp) {
		for k, v := range cc.Modifiers(comp) {
			merged[k] = v
		}
	}

	return merged
}

func (cc *ComponentContainer) decorateComponents(decorators map[string]ComponentDecorator) {

	for _, component := range cc.allComponents {
		for _, decorator := range decorators {

			if decorator.OfInterest(component) {
				decorator.DecorateComponent(component, cc)
			}
		}
	}

	for n, _ := range decorators {
		delete(cc.allComponents, n)
	}
}

func (cc *ComponentContainer) captureDecorator(component *Component, decorators map[string]ComponentDecorator) {

	decorator, isDecorator := component.Instance.(ComponentDecorator)

	if isDecorator {
		cc.FrameworkLogger.LogTracef("Found decorator %s", component.Name)
		decorators[component.Name] = decorator
	}
}

func (cc *ComponentContainer) addComponent(component *Component) {
	cc.allComponents[component.Name] = component

	l := cc.FrameworkLogger

	n, nameable := component.Instance.(ComponentNamer)

	if nameable {
		n.SetComponentName(component.Name)
	}

	_, startable := component.Instance.(Startable)

	if startable {
		l.LogTracef("%s is Startable", component.Name)
		cc.startable = append(cc.startable, component)
	}

	_, stoppable := component.Instance.(Stoppable)

	if stoppable {
		l.LogTracef("%s is Stoppable", component.Name)
		cc.stoppable = append(cc.stoppable, component)
	}

	_, blocker := component.Instance.(AccessibilityBlocker)

	if blocker {
		l.LogTracef("%s is an AvailabilityBlocker", component.Name)
		cc.blocker = append(cc.blocker, component)
	}

	_, accessible := component.Instance.(Accessible)

	if accessible {
		l.LogTracef("%s is a Accesible", component.Name)
		cc.accessible = append(cc.accessible, component)
	}

}
