<?php

namespace FlowCatalyst\Generated\Endpoint;

class PutApiSdkRoleByRoleName extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $roleName;
    /**
     * @param string $roleName
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutBody $requestBody
     */
    public function __construct(string $roleName, ?\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutBody $requestBody = null)
    {
        $this->roleName = $roleName;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{roleName}'], [$this->roleName], '/api/sdk/roles/{roleName}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutBody) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkRoleByRoleNameBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkRoleByRoleNameNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse200', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiSdkRoleByRoleNameBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse400', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\PutApiSdkRoleByRoleNameNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}